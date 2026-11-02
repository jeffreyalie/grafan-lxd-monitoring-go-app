package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	vault "github.com/hashicorp/vault/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	interval := 30 * time.Second
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	prometheus.MustRegister(newLXDCollector(interval))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("Metrics server starting on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func fetchInstances() ([]api.InstanceFull, error) {
	cert, key, err := getSecrets()
	if err != nil {
		return nil, fmt.Errorf("Vault Error: %w", err)
	}
	client, err := lxd.ConnectLXD(os.Getenv("LXD_ADDR"), &lxd.ConnectionArgs{
		TLSClientCert:      cert,
		TLSClientKey:       key,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("LXD Connection Error: %w", err)
	}
	instances, err := client.UseProject("default").GetInstancesFull(
		lxd.GetInstancesFullArgs{InstanceType: api.InstanceTypeAny})
	if err != nil {
		return nil, fmt.Errorf("LXD Query Error: %w", err)
	}
	return instances, nil
}

func getSecrets() (cert, key string, err error) {
	config := vault.DefaultConfig()
	config.Address = os.Getenv("VAULT_ADDR")

	if os.Getenv("VAULT_SKIP_VERIFY") == "true" {
		_ = config.ConfigureTLS(&vault.TLSConfig{Insecure: true})
	}

	client, err := vault.NewClient(config)
	if err != nil {
		return "", "", err
	}

	resp, err := client.Logical().Write("auth/approle/login", map[string]interface{}{
		"role_id":   os.Getenv("VAULT_ROLE_ID"),
		"secret_id": os.Getenv("VAULT_SECRET_ID"),
	})
	if err != nil || resp == nil || resp.Auth == nil {
		return "", "", fmt.Errorf("vault login failed")
	}
	client.SetToken(resp.Auth.ClientToken)

	secret, err := client.Logical().Read("homelab/data/lxd")
	if err != nil || secret == nil {
		return "", "", fmt.Errorf("failed to read secrets")
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("unexpected secret format")
	}
	cert, _ = data["client_cert"].(string)
	key, _ = data["client_key"].(string)
	if cert == "" || key == "" {
		return "", "", fmt.Errorf("client_cert/client_key missing in secret")
	}
	return cert, key, nil
}
