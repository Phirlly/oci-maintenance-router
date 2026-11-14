package configuration

// DecodeCatalog reads the independently trusted deployment catalog.
func DecodeCatalog(data []byte) (Catalog, error) {
	value, err := strictJSON(data)
	if err != nil {
		return Catalog{}, err
	}
	reader := readObject(value, "catalog", &err, []string{"regions", "mysqlServices"}, nil)
	result := Catalog{Regions: reader.stringMap("regions"), MySQLServices: reader.strings("mysqlServices")}
	if err != nil {
		return Catalog{}, err
	}
	if err := validateCatalog(result); err != nil {
		return Catalog{}, err
	}
	return result, nil
}
