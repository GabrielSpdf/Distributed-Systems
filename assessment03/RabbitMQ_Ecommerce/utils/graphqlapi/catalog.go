package graphqlapi

import "github.com/graph-gophers/graphql-go"

const CatalogSchema = `
	type Product {
		id: String!
		name: String!
		category: String!
		price: Float!
		quantity: Int!
	}

	type ProductList {
		products: [Product!]!
		total: Int!
	}
`

type CatalogArgs struct {
	Category  *string
	Available graphql.NullBool
}

type Product struct {
	ID       string
	Name     string
	Category string
	Price    float64
	Quantity int32
}

type ProductList struct {
	Products []Product
	Total    int32
}
