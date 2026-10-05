resource "americancloud_object_storage_unit" "media" {
  name = "mediaassets"
}

# One key for each application, so you can rotate one without touching the others.
resource "americancloud_object_storage_access_key" "uploader" {
  storage_unit_id = americancloud_object_storage_unit.media.id
  label           = "uploader"
}

# The S3 credentials of the key:
#   americancloud_object_storage_access_key.uploader.access_key
#   americancloud_object_storage_access_key.uploader.secret_key (sensitive)
