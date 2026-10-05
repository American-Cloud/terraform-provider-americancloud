# The import ID is "<storage_unit_id>/<access_key>". The unit's original key cannot be
# imported here: it is the access_key/secret_key of americancloud_object_storage_unit.
terraform import americancloud_object_storage_access_key.uploader 'tenant$mediaassets/AKIAEXAMPLEKEY000000'
