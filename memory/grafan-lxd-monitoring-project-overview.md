---
name: grafan-lxd-monitoring-project-overview
description: Overview of the Grafan LXD monitoring project - a Go exporter that exposes LXD instance metrics as Prometheus metrics
metadata:
  type: project
---

This repository implements a complete infrastructure-as-code solution that:
- Provisions Ubuntu 24.04 VMs on LXD using Terraform with a reusable lxd-vm module
- Configures VMs with cloud-init for automated setup
- Deploys applications via Ansible playbooks and local roles
- Manages environments (dev/prod) with separate state management per VM in MinIO
- Provides CI/CD automation through local Gitea Actions workflows

The specific application is a lightweight Go exporter that exposes the live state of LXD instances (VMs and containers) as Prometheus metrics, with a ServiceMonitor and a Grafana dashboard shipped in the Helm chart.

Important Commands:
Terraform
# Validate Terraform configuration
terraform validate

# Format all .tf files
terraform fmt -check -diff

# Initialize a Terraform environment (example for dev)
cd terraform/env/dev
terraform init -backend-config="endpoint=http://minio-host:9000" \
  -backend-config="bucket=terraform-state" \
  -backend-config="key=state/dev/dev01/terraform.tfstate" \
  -backend-config="region=us-east-1" \
  -backend-config="access_key=<key>" \
  -backend-config="secret_key=<secret>" \
  -backend-config="skip_credentials_validation=true" \
  -backend-config="skip_metadata_api_check=true" \
  -backend-config="skip_requesting_account_id=true" \
  -backend-config="force_path_style=true"

# Plan Terraform changes
terraform plan -var-file="dev01.tfvars"

# Apply Terraform changes
terraform apply -auto-approve -var-file="dev01.tfvars"

# Show Terraform outputs
terraform output vm_ip

# Destroy a VM
terraform destroy -var-file="dev01.tfvars"

Ansible
# Run ansible-lint on playbook
ansible-lint ansible/playbook.yml

# Validate ansible playbook syntax
ansible-playbook ansible/playbook.yml --syntax-check

# Run playbook manually (example)
VM_IP=192.168.1.100
ansible-playbook -i "$VM_IP," \
  -u ansible \
  --private-key ~/.ssh/id_rsa \
  ansible/playbook.yml

Investigation Tips:
- Terraform: 
- Use terraform state list to list all resources.
- Use terraform state show <resource> to view details of a specific resource.
- The lxd-vm module is the core component - look at terraform/modules/lxd-vm/
- Ansible:
- Roles are in ansible/roles/
- Playbook is ansible/playbook.yml
- Inventory is dynamic, set via environment variable

Constraints:
1. Version Compatibility: 
- Terraform ≥ 1.5 required
- LXD installed via snap on Ubuntu 24.04 host
2. Secrets Management:
- All secrets configured at Gitea org level (Infra org) for workflows
- Secrets must be set in repository settings:
- LXD_ADDRESS, LXD_CLIENT_CERT, LXD_CLIENT_KEY
- MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY
- ANSIBLE_SSH_PUBLIC_KEY, ANSIBLE_SSH_PRIVATE_KEY
3. Deployment Flow:
- Terraform first creates the VM, then Ansible configures it
- VM name convention: <environment>-<os_type>-<vm_name>
- Examples: dev-ubuntu-vm-dev01, prod-ubuntu-vm-prod02
- State is stored in MinIO at path: s3://terraform-state/state/<environment>/<vm_name>/terraform.tfstate
4. Security:
- SSH hardening enabled in cloud-init template
- Password authentication disabled for ansible user
- SSH keys used for both Terraform LXD connections and Ansible deployment
5. Workflow Requirements:
- All workflows run from the root directory of the repository
- Workflows use Gitea Actions with predefined environment variables from secrets
- Workflow inputs must match specific patterns (environment = dev/prod, vm_name = <name>.tfvars)