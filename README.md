# Go-Matchmaker
[![Coverage](https://github.com/st-matskevich/go-matchmaker/wiki/coverage.svg)](https://raw.githack.com/wiki/st-matskevich/go-matchmaker/coverage.html)
[![Go Report](https://goreportcard.com/badge/github.com/st-matskevich/go-matchmaker)](https://goreportcard.com/report/github.com/st-matskevich/go-matchmaker)
[![License](https://img.shields.io/github/license/st-matskevich/go-matchmaker)](LICENSE)


Microservices based orchestrator for your containers written in Go. Can be used to orchestrate game servers or virtual machines.

## Installation

1. Install [Docker](https://docs.docker.com/get-docker/)
2. Check variables in [docker-compose.yml](docker-compose.yml)
```yml
# API service
# How much service will wait for Reservation API confirmation from created server in ms
RESERVATION_TIMEOUT: 5000

# Maker service
# How much service will wait for Reservation API confirmation from created server in ms
RESERVATION_TIMEOUT: 5000
# How much threads should be created for requsets processing
MAX_CONCURRENT_JOBS: 3
# Docker network that will be used for starting new containers
DOCKER_NETWORK: dev-network
# How much thread should wait between looking for available containers
LOOKUP_COOLDOWN: 1000

# Network for compose is not created automaticaly
# go-matchmaker containers and your servers should run on same network to be able to interact with each other
# Change dev-network to your network if you wish
# It should be equal to DOCKER_NETWORK variable
networks:
  dev-network:
    name: dev-network
```
3. Setup secrets and additional environment variables(or create .env file)
```properties
# Image to use as server container
IMAGE_TO_PULL=docker.io/stmatskevich/go-dummyserver
# Image port that should be exposed, protocol can be also specified, tcp is used if nothing added
IMAGE_EXPOSE_PORT=3000/tcp
# Image port that provide Reservation API
IMAGE_CONTROL_PORT=3000
# Image registry username, if authorization not needed leave blank
IMAGE_REGISTRY_USERNAME=stmatskevich
# Image registry password, if authorization not needed leave blank
IMAGE_REGISTRY_PASSWORD=supersecretpassword
```
4. Create Docker network that was defined as `DOCKER_NETWORK` in [docker-compose.yml](docker-compose.yml). Recommendation: use bridge driver to avoid exposing excess ports
```sh
docker network create -d bridge dev-network  
```
5. Run docker compose from root directory
```sh
docker compose up --build -d
```
### Optional
 * Use proc/sys/net/ipv4/ip_local_port_range to limit number of ports that will be used for exposing

## Reservation API

To use your own image with go-matchmaker, it should serve <b>Reservation API</b> on `IMAGE_CONTROL_PORT` port.

### Reservation API Endpoints

#### <code>POST <b>/reservation/{client-id}</b></code>
Used from <b>Maker</b> service to reserve slot for client with <code>client-id</code> id.

Respond with `200` if slot was successfully reserved, `403` otherwise.

#### <code>GET <b>/reservation/{client-id}</b></code>
Used from <b>API</b> service to verify reservation for client with <code>client-id</code> id.

Respond with `200` if there is a slot reserved for specified client, `404` otherwise.

You can use [go-dummyserver](https://github.com/st-matskevich/go-dummyserver) as example or image for testing. Available as [image](https://hub.docker.com/r/stmatskevich/go-dummyserver) on Docker Hub. 


## Clients authentication

No special authentication included by default, but all interfaces are already here for you.

To use authentication you need to implement your own type with <code>Authorize</code> method form <code>[Authorizer](api/auth/auth.go)</code> interface. Then pass your type to <code>auth.New()</code> middleware in <code>[api/main.go](api/main.go)</code>.

You can use <code>[DummyAuthorizer](api/auth/auth.go)</code> as example.

## Usage

To request server send <code>POST <b>/request</b></code> request with authorization token:
```sh
curl -X POST http://localhost:3000/request -H "Authorization: 5jg86j39jdf04"
```

To view services logs use:
```sh
# API service
docker compose logs api -f

# Maker service
docker compose logs maker -f
```

## Documentation

See [DOCUMENTATION](DOCUMENTATION.md) for more information.

## Built with

- [Go](https://go.dev/)
- [Docker](https://www.docker.com/)
- [Redis](https://github.com/redis/redis)
- [Moby Project](https://github.com/moby/moby)
- [Fiber](https://github.com/gofiber/fiber)
- [go-redis](https://github.com/redis/go-redis)
- [testify](https://github.com/stretchr/testify)
- [godotenv](https://github.com/joho/godotenv)

## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.

## Contributing

Want a new feature added? Found a bug?
Go ahead an open [a new issue](https://github.com/st-matskevich/go-matchmaker/issues/new) or feel free to submit a pull request.