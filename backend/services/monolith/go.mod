module github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith

go 1.27.0

require (
	github.com/mothercat-enterprise/mothercat/backend/services/access v0.0.0
	github.com/mothercat-enterprise/mothercat/backend/services/audit v0.0.0
	github.com/mothercat-enterprise/mothercat/backend/services/identity v0.0.0
	github.com/mothercat-enterprise/mothercat/backend/services/organization v0.0.0
	go.mongodb.org/mongo-driver v1.17.6
)

require (
	github.com/golang/snappy v0.0.4 // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/mothercat-enterprise/mothercat/backend/services/identity => ../identity

replace github.com/mothercat-enterprise/mothercat/backend/services/organization => ../organization

replace github.com/mothercat-enterprise/mothercat/backend/services/access => ../access

replace github.com/mothercat-enterprise/mothercat/backend/services/audit => ../audit
