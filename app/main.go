package main

import (
	"fmt"
	"html/template"
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

type InstanceData struct {
	Instances []api.InstanceFull
	Error     error
}

func main() {
	prometheus.MustRegister(newLXDCollector(30 * time.Second))

	http.HandleFunc("/", statusHandler)
	http.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server starting on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	instances, err := fetchInstances()
	data := InstanceData{Instances: instances, Error: err}
	renderTemplate(w, data)
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

func renderTemplate(w http.ResponseWriter, data InstanceData) {
	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		http.Error(w, "Template Load Error", http.StatusInternalServerError)
		return
	}
	_ = tmpl.Execute(w, data)
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

	loginData := map[string]interface{}{
		"role_id":   os.Getenv("VAULT_ROLE_ID"),
		"secret_id": os.Getenv("VAULT_SECRET_ID"),
	}
	resp, err := client.Logical().Write("auth/approle/login", loginData)
	if err != nil || resp == nil {
		return "", "", fmt.Errorf("Vault login failed")
	}
	client.SetToken(resp.Auth.ClientToken)

	secret, err := client.Logical().Read("homelab/data/lxd")
	if err != nil || secret == nil {
		return "", "", fmt.Errorf("failed to read secrets")
	}

	secretData := secret.Data["data"].(map[string]interface{})
	return secretData["client_cert"].(string), secretData["client_key"].(string), nil
}
