# Go Task Scheduler

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A robust, distributed task scheduler and worker system built with Go. This application provides a RESTful API to schedule tasks for future execution, which are then picked up and processed by a resilient background worker.

## Overview

The system allows for scheduling background tasks through a RESTful API. Tasks are defined by an **Entity** (e.g., `PRODUCT`) and an **Action** (e.g., `CREATE`), providing a flexible and extensible model. Scheduled tasks are stored in a PostgreSQL database for durability and queued in Redis for efficient processing by a distributed worker system.

## Features

- **RESTful API**: Schedule and manage tasks using a clean HTTP interface built with Gin.
- **Normalized Task Model**: Tasks are defined by `Entities` and `Actions` stored in the database, preventing magic strings and allowing for easy extension.
- **Background Worker**: A concurrent worker processes tasks from a queue, with configurable concurrency.
- **Distributed Locking**: Uses Redis `SETNX` to ensure that a task is processed by only one worker instance at a time, making it safe to scale horizontally.
- **Automatic Retries**: Failed tasks are automatically retried with exponential backoff, up to a configurable maximum number of attempts.
- **Task Prioritization**: Assign priorities to tasks to ensure high-priority jobs are processed first.
- **Resilient**: On startup, the worker reconciles tasks that may have been stuck in a `processing` state due to a previous crash, ensuring no tasks are lost.
- **Configurable**: Worker behavior (concurrency, polling interval) can be configured via environment variables.
- **Database-backed**: Tasks are persisted in PostgreSQL for durability and querying.
- **Granular Statuses**: Tasks have a clear lifecycle with statuses like `pending`, `processing`, `completed`, `failed`, `paused`, and `canceled`.
- **API Documentation**: Interactive API documentation is available via Swagger (OpenAPI).
- **Containerized**: The entire application stack (app, database, Redis) is managed with Docker Compose for easy setup and deployment.

## Tech Stack

- **Language**: Go
- **Web Framework**: Gin
- **Database**: PostgreSQL
- **ORM**: GORM
- **In-Memory Store/Queue**: Redis
- **Containerization**: Docker & Docker Compose
- **API Documentation**: Swaggo

## Getting Started

### Prerequisites

- Go (v1.18+ recommended)
- Docker
- Docker Compose

### 1. Clone the Repository

```sh
git clone https://github.com/rizqitaufiqf/go-scheduler.git
cd go-scheduler
```

### 2. Configure Environment Variables

Create a `.env` file by copying the example file.

```sh
cp .env.example .env
```

Modify the `.env` file with your desired configuration. The default values are suitable for local development with the provided `docker-compose.yml`.

### 3. Run the Application

Use Docker Compose to build and run all the services (Go application, PostgreSQL, and Redis).

```sh
docker-compose up --build
```

The API server will be running at `http://localhost:8080`.

## API Usage

### Interactive Documentation

Once the application is running, you can access the interactive Swagger UI to explore and test the API endpoints:

**http://localhost:8080/swagger/index.html**

### Using with Postman / Insomnia

You can easily import the API collection into tools like Postman or Insomnia using the generated OpenAPI specification.

1.  Make sure the application is running:
    ```sh
    docker-compose up
    ```
2.  Open Postman and go to `File > Import`.
3.  Select the **Link** tab and enter the URL to the `swagger.json` file:
    ```
    http://localhost:8080/swagger/swagger.json
    ```
4.  Click **Continue** and then **Import**.

A new collection named "Go Task Scheduler API" will be created, containing all available endpoints and their documentation. You can then use Postman to send requests and see responses.

> **Important Note on Timezones:** When scheduling a task, always provide the `scheduled_at` timestamp in full **ISO 8601 / RFC3339 format**, including the timezone offset. For example, to schedule a task for 10:00 AM in a timezone that is 7 hours ahead of UTC, use `"2025-10-20T10:00:00+07:00"`. If you omit the offset, the time will be interpreted as UTC.

### Example: Scheduling a Task with `curl`

Here's how to schedule a task to create a new product in 2 minutes. This example uses UTC time.

```sh
# Get a timestamp for 2 minutes from now (Linux/GNU date)
SCHEDULED_AT=$(date -d "+2 minutes" -u +"%Y-%m-%dT%H:%M:%SZ")

# For macOS, use:
# SCHEDULED_AT=$(date -v+2M -u +"%Y-%m-%dT%H:%M:%SZ")

# Schedule the task
curl -X POST http://localhost:8080/api/v1/scheduler/products/create \
-H "Content-Type: application/json" \
-d '{
    "scheduled_at": "'"$SCHEDULED_AT"'",
    "priority": 10,
    "max_retries": 5,
    "payload": {
        "name": "New Awesome Gadget",
        "price": 199.99,
        "stock": 50
    }
}'
```

You will see the worker logs in your `docker-compose` output when the task is picked up and processed.

### Viewing Tasks

You can view all scheduled tasks and their statuses:

```sh
curl http://localhost:8080/api/v1/scheduler/tasks
```

## Project Structure

```
├── config/         # Environment configuration loading
├── database/       # Database and Redis initialization
├── docs/           # Swagger documentation files (auto-generated)
├── dto/            # Data Transfer Objects (for API and database models)
├── handler/        # HTTP handlers for API endpoints
├── repository/     # Data access logic (e.g., scheduling a task)
├── router/         # Gin router setup
├── worker/         # Background worker and task processing logic
├── .env.example    # Example environment variables
├── docker-compose.yml # Docker services definition
├── Dockerfile      # Docker build instructions for the Go app
├── go.mod          # Go module dependencies
└── main.go         # Application entrypoint
```

## Configuration

The following environment variables can be set in the `.env` file:

| Variable                       | Description                                       | Default |
| ------------------------------ | ------------------------------------------------- | ------- |
| `POSTGRES_USER`                | PostgreSQL username.                              | `user`    |
| `POSTGRES_PASSWORD`            | PostgreSQL password.                              | `password`  |
| `POSTGRES_DB`                  | PostgreSQL database name.                         | `scheduler_db`|
| `POSTGRES_HOST`                | Hostname for the PostgreSQL server.               | `postgresql`|
| `POSTGRES_PORT`                | Port for the PostgreSQL server.                   | `5432`    |
| `REDIS_ADDR`                   | Address for the Redis server.                     | `redis:6379`|
| `TZ`                           | Timezone for the application.                     | `Asia/Jakarta`|
| `WORKER_CONCURRENCY`           | Max number of tasks the worker can run at once.   | `10`      |
| `WORKER_POLL_INTERVAL_SECONDS` | How often (in seconds) the worker polls for tasks. | `10`      |

## License

This project is licensed under the Apache 2.0 License. See the LICENSE file for details.