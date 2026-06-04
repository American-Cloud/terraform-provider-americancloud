resource "americancloud_object_storage_unit" "media" {
  name        = "mediaassets"
  max_size_gb = 100 # omit for unlimited
}

# The unit's S3 credentials are computed, sensitive outputs:
#   americancloud_object_storage_unit.media.access_key
#   americancloud_object_storage_unit.media.secret_key
# Use them with any S3 client against the endpoint:
#   https://a2-west.americancloud.com
