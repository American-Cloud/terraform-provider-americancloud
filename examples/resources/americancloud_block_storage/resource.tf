resource "americancloud_block_storage" "data" {
  name    = "data-volume"
  size_gb = 20
  region  = "us-west-0"
}
