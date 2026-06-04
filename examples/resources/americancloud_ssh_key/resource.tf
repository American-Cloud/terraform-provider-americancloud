# Register an existing public key
resource "americancloud_ssh_key" "laptop" {
  name       = "my-laptop"
  public_key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host"
}

# Or omit public_key to have a pair generated (private_key is returned once)
resource "americancloud_ssh_key" "generated" {
  name = "ci-deploy"
}
