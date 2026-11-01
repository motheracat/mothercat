module github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith

go 1.27.0

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/jackc/pgx/v5 v5.12.0
	github.com/mothercat-enterprise/mothercat/backend/services/access v0.0.0
	github.com/mothercat-enterprise/mothercat/backend/services/audit v0.0.0
	github.com/mothercat-enterprise/mothercat/backend/services/identity v0.0.0
	github.com/mothercat-enterprise/mothercat/backend/services/organization v0.0.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/mothercat-enterprise/mothercat/backend/services/identity => ../identity

replace github.com/mothercat-enterprise/mothercat/backend/services/organization => ../organization

replace github.com/mothercat-enterprise/mothercat/backend/services/access => ../access

replace github.com/mothercat-enterprise/mothercat/backend/services/audit => ../audit
