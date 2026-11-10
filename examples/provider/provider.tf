terraform {
  required_providers {
    americancloud = {
      source = "American-Cloud/americancloud"
    }
  }
}

provider "americancloud" {
  # Credentials may be set here or via the environment variable
  #   AMERICANCLOUD_API_TOKEN
  # api_token = "amcld_..."
  # api_url           = "https://api.americancloud.com" # optional override
}
