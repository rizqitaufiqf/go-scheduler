# Define variables for the compose files
DEV_COMPOSE := docker-compose.dev.yml
PROD_COMPOSE := docker-compose.prod.yml
DOCKER_CONTAINER_NAME := go-scheduler-api
DOCKER_CONTAINER_WORKER_NAME := go-scheduler-worker

.PHONY: help swag build-dev run-dev down-dev clean-dev logs-dev restart-dev shell-dev build-prod run-prod down-prod clean-prod logs-prod restart-prod shell-prod

help: ## ✨ Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

swag: ## 📚 Generate Swagger Documentations
	docker exec -it $(DOCKER_CONTAINER_NAME) swag init --parseDependency --parseInternal

# --- Development Commands ---
build-dev: ## 📦 Build the development environment
	docker-compose -f $(DEV_COMPOSE) build

run-dev: ## 🚀 Start the development environment (with hot-reloading)
	docker-compose -f $(DEV_COMPOSE) up --build -d

down-dev: ## ⛔ Stop the development environment
	docker-compose -f $(DEV_COMPOSE) down

clean-dev: ## 🧹 Fully clean the dev environment (removes local volumes and images)
	docker-compose -f $(DEV_COMPOSE) down --volumes --rmi local

restart-dev: ## 🔄 Restart the development environment (rebuilds images)
	make clean-dev
	make run-dev

logs-dev: ## 📝 Show the logs of the development environment
	docker logs -f --tail 100 $(DOCKER_CONTAINER_NAME)
	
logs-dev-worker: ## 📝 Show the logs of the development environment
	docker logs -f --tail 100 $(DOCKER_CONTAINER_WORKER_NAME)

shell-dev: ## 🐚 Start an interactive shell inside the dev container
	docker exec -it $(DOCKER_CONTAINER_NAME) /bin/bash

# --- Production Commands ---
build-prod: ## 📦 Build the production environment
	docker-compose -f $(PROD_COMPOSE) build

run-prod: ## 🚢 Deploy the production environment
	docker-compose -f $(PROD_COMPOSE) up --build -d

down-prod: ## 🛑 Stop the production environment
	docker-compose -f $(PROD_COMPOSE) down

clean-prod: ## 🗑️ Fully clean the prod environment (removes local volumes and images)
	docker-compose -f $(PROD_COMPOSE) down --volumes --rmi local

restart-prod: ## ♻️ Restart the production environment (rebuilds images)
	make clean-prod
	make run-prod

logs-prod: ## 📝 Show the logs of the production environment
	docker logs -f --tail 100 scheduler-app-prod

shell-prod: ## 🐚 Start an interactive shell inside the prod container
	docker exec -it scheduler-app-prod /bin/bash
