.PHONY: db-up db-down db-status db-logs

db-up:
	docker compose up --detach postgres

db-down:
	docker compose down

db-status:
	docker compose ps

db-logs:
	docker compose logs --follow postgres
