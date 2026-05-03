package aliases

var (
	GenreConfig = ResourceConfig{
		AliasTable:    "genre_aliases",
		ResourceFK:    "genre_id",
		ResourceTable: "genres",
	}
	TagConfig = ResourceConfig{
		AliasTable:    "tag_aliases",
		ResourceFK:    "tag_id",
		ResourceTable: "tags",
	}
	SeriesConfig = ResourceConfig{
		AliasTable:    "series_aliases",
		ResourceFK:    "series_id",
		ResourceTable: "series",
	}
	PersonConfig = ResourceConfig{
		AliasTable:    "person_aliases",
		ResourceFK:    "person_id",
		ResourceTable: "persons",
	}
	PublisherConfig = ResourceConfig{
		AliasTable:    "publisher_aliases",
		ResourceFK:    "publisher_id",
		ResourceTable: "publishers",
	}
	ImprintConfig = ResourceConfig{
		AliasTable:    "imprint_aliases",
		ResourceFK:    "imprint_id",
		ResourceTable: "imprints",
	}
)
