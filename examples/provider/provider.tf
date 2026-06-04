terraform {
  required_providers {
    americancloud = {
      source = "American-Cloud/americancloud"
    }
  }
}

provider "americancloud" {
  # Credentials may be set here or via environment variables:
  #   AMERICANCLOUD_API_CLIENT_ID, AMERICANCLOUD_API_CLIENT_SECRET
  # api_client_id     = "..."
  # api_client_secret = "..."
  # api_url           = "https://api.americancloud.com" # optional override
}
