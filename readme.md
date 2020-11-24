This app reads from an Ansible Tower metrics endpoint (`api/v2/metrics`) and scrapes the metrics into StatsD metrics.

## Usage
`Usage: ./goScrapeAnsibleMetrics -api-token={} -server-url={}`
* **api-token**: The Ansible Tower token to pull metrics
* **server-url**: The Ansible Tower server to pull metrics from

### Assumptions
* This will send the metrics to localhost on port 18125
* Port 443 is expected for the Ansible UI
* You must generate a token which can query the metrics

### Dev Build Flag
To build for Linux systems
`env GOOS=linux GOARCH=386 go build -o goScrapeAnsibleStatsD main.go`