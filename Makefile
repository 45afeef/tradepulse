# OPTIONAL shortcuts for Unix/macOS. Everything is plain `docker compose`; Windows users
# can run the docker compose commands directly (see README).
.PHONY: dev check test openapi tidy

dev:
	docker compose up --build

check:
	docker compose run --rm check

test:
	docker compose run --rm test

openapi:
	docker compose run --rm openapi

tidy:
	docker compose run --rm tidy
