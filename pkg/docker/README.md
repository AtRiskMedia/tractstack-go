# TractStack Sandbox Docker Appliance

This guide provides instructions for building, running, and managing the TractStack sandbox environment using Docker.

## First-Time Setup

Follow these steps to build the image and run the container for the first time.

### 1\. Build the Docker Image

This command builds the Docker image from the `Dockerfile`. It only needs to be run once, or whenever you make changes to the `Dockerfile` or related scripts.

```bash
docker build -t tractstack-sandbox .
```

### 2\. Run the Container

To run the application as a background server process (recommended), use the following command. The application will start and initialize automatically.

```bash
docker run -d --name my-tractstack-sandbox -p 4321:4321 -p 8080:8080 -v tractstack_data:/home/sandbox/t8k/t8k-go-server tractstack-sandbox
```

**Explanation of Flags:**

* `-d`: Detached mode. Runs the container in the background and prints the container ID.
* `--name my-tractstack-sandbox`: Assigns a consistent, easy-to-remember name to your container.
* `-p 4321:4321`: Maps the container's port 4321 (Astro frontend) to your local machine's port 4321.
* `-p 8080:8080`: Maps the container's port 8080 (Go backend) to your local machine's port 8080.
* `-v tractstack_data:/home/sandbox/t8k/t8k-go-server`: Creates a persistent, named data volume called `tractstack_data` to store your database and configuration.

After running this, the application will be available at `http://localhost:4321`.

## Managing the Container

Once the container is running in the background, use these commands to manage it.

### Viewing Logs

To view the live logs from the application (both frontend and backend):

```bash
docker logs -f my-tractstack-sandbox
```

> Press `CTRL+C` to stop viewing the logs. This will **not** stop the container.

### Stopping, Starting, and Restarting

```bash
# To stop the running container
docker stop my-tractstack-sandbox

# To start the container again (retains all data)
docker start my-tractstack-sandbox

# To restart the container
docker restart my-tractstack-sandbox
```

## Cleanup and Full Reset

These commands are for when you want to remove resources.

### Removing the Container

This removes the container but **leaves your data volume intact**.

```bash
# First, stop the container if it's running
docker stop my-tractstack-sandbox

# Then, remove it
docker rm my-tractstack-sandbox
```

### Performing a Full Reset (Destroying All Data)

This is a destructive action that will completely wipe all databases, configurations, and tenant data, returning the application to a factory-fresh state on the next run.

> **Warning:** This operation cannot be undone.

```bash
docker volume rm tractstack_data
```
