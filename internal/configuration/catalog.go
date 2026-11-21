package configuration

import "encoding/json"

// DecodeCatalog reads the independently trusted deployment catalog.
func DecodeCatalog(data []byte) (Catalog, error) {
	value, err := strictJSON(data)
	if err != nil {
		return Catalog{}, err
	}
	fields, _ := value.(map[string]any)
	var result Catalog
	if version, exists := fields["schemaVersion"]; exists {
		if version != json.Number("1") {
			return Catalog{}, &Error{"catalog.schemaVersion", "unsupported"}
		}
		reader := readObject(value, "catalog", &err, []string{"schemaVersion", "regions", "resourceServices"}, nil)
		services := readObject(reader.values["resourceServices"], "catalog.resourceServices", &err, []string{resourceMySQL, resourceInstance}, nil)
		result = Catalog{SchemaVersion: 1, Regions: reader.stringMap("regions"), ResourceServices: map[string][]string{
			resourceMySQL: services.strings(resourceMySQL), resourceInstance: services.strings(resourceInstance),
		}}
	} else {
		reader := readObject(value, "catalog", &err, []string{"regions", "mysqlServices"}, nil)
		result = Catalog{Regions: reader.stringMap("regions"), MySQLServices: reader.strings("mysqlServices")}
	}
	if err != nil {
		return Catalog{}, err
	}
	if err := validateCatalog(result); err != nil {
		return Catalog{}, err
	}
	return result, nil
}
