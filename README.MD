# Job Item Worker

A distributed worker application built in Go that executes jobs triggered by a Job Manager through message brokers. This worker runs on individual nodes and listens for job execution commands from a centralized job management system.

## Overview

Job Item Worker is a lightweight, configurable worker service designed to:
- Receive job execution triggers from a Job Manager
- Execute predefined commands/scripts based on job events
- Support multiple message broker protocols (NATS, RabbitMQ, Redis)
- Provide real-time status updates and notifications
- Handle hardware monitoring and reporting
- Support TLS/SSL secure connections
- Auto-update capabilities for seamless deployments

## Architecture

```
┌─────────────────┐    Message Broker     ┌─────────────────┐
│   Job Manager   │◄────────────────────►│   Job Item      │
│   (Central)     │   (NATS/AMQP/Redis)   │   (Worker Node) │
└─────────────────┘                       └─────────────────┘
                                                    │
                                                    ▼
                                          ┌─────────────────┐
                                          │  Job Execution  │
                                          │  (Commands/     │
                                          │   Scripts)      │
                                          └─────────────────┘
```

## Features

### Core Functionality
- **Message-driven Job Execution**: Listens for job triggers via message brokers
- **Multi-broker Support**: NATS, RabbitMQ (AMQP), and Redis pub/sub
- **Template-based Commands**: Dynamic command generation using Mustache templates
- **Process Management**: Graceful startup, shutdown, and restart capabilities
- **Configuration Hot-reload**: Automatic restart on configuration changes
- **Pipeline Integration**: Save command to capture and forward command output as notifications

### Monitoring & Reporting
- **Hardware Information**: CPU, memory, disk, and network monitoring
- **Real-time Status**: Job execution status and progress reporting
- **Event Bus**: Internal event system for component communication
- **REST API**: HTTP endpoints for local job management and notifications

### Security & Reliability
- **TLS/SSL Support**: Secure broker connections with certificate authentication
- **Auto-update**: Automatic version checking and binary updates
- **Process Supervision**: Parent-child process architecture for reliability
- **Signal Handling**: Graceful shutdown on system signals

## Installation

### Prerequisites
- Go 1.24.1 or higher
- Access to a message broker (NATS, RabbitMQ, or Redis)
- Linux/Windows/macOS operating system

### Build from Source
```bash
# Clone the repository
git clone <repository-url>
cd job_item

# Build the application
chmod +x build.sh
./build.sh

# Or build manually
go build -o job_item main.go
```

### Binary Installation
Download the pre-built binary for your platform from the releases page.

## Configuration

### Basic Configuration (`config.yaml`)
```yaml
# Unique identifier for this worker instance
identity_id: "worker-node-01"

# Job Manager endpoint for configuration updates
end_point: "http://job-manager:5280/api/worker/config"

# Authentication credentials
credential:
  project_id: "7bd0c868-87a0-4c90-8d27-559677763bb6"
  secret_key: "your-secret-key"

# Job definitions
jobs:
  - name: "Document Validation"
    event: "validate_doc_and_style"
    cmd: "python validate.py {{task_id}}.json"
  
  - name: "Email Notification"
    event: "send_email"
    cmd: "node emailer.js {{task_id}}.json"
  
  - name: "Logo Processing"
    event: "validate_logo"
    cmd: "./logo_processor {{task_id}}.json"

# Background processes (optional)
execs:
  - name: "Log Monitor"
    key: "log_monitor"
    cmd: "tail -f /var/log/app.log"
    working_dir: "/var/log"
    cascade_exit: true
    attempt: 3
    env:
      LOG_LEVEL: "INFO"
```

### Broker Configuration Examples

#### NATS Configuration
```yaml
# Automatically configured via Job Manager endpoint
# Supports:
# - Username/password authentication
# - Token-based authentication  
# - TLS/SSL with certificates
# - Clustered NATS servers
```

#### RabbitMQ Configuration
```yaml
# Automatically configured via Job Manager endpoint
# Supports:
# - AMQP 0.9.1 protocol
# - SSL/TLS connections
# - Exchange-based routing
# - Durable queues
```

#### Redis Configuration
```yaml
# Automatically configured via Job Manager endpoint
# Supports:
# - Redis pub/sub
# - Password authentication
# - SSL/TLS connections
# - Multiple databases
```

## Usage

### Starting the Worker
```bash
# Start with default config
./job_item

# Start with custom config
./job_item --config=/path/to/config.yaml

# Start with environment variable
CONFIG_PATH=/path/to/config.yaml ./job_item
```

### Command Line Interface
```bash
# Main process (manages child processes)
./job_item --config=config.yaml

# Child process (actual worker)
./job_item child_process --config=config.yaml

# Exec process (background tasks)
./job_item child_execs_process --config=config.yaml

# Save command (capture piped input and send as notification)
./job_item save
```

### Save Command

The `save` command allows you to capture output from other commands and send it as notifications to the Job Manager. This is useful for logging command outputs, monitoring scripts, or integrating with existing shell workflows.

#### Usage
```bash
# Capture ping output and send as notification
ping google.com -c 5 | ./job_item save

# Capture log file content
tail -n 100 /var/log/app.log | ./job_item save

# Capture command output with error handling
ls -la /some/directory 2>&1 | ./job_item save

# Capture script output
./monitoring_script.sh | ./job_item save
```

#### Required Environment Variables
The `save` command requires two environment variables to be set:

- **`JOB_ITEM_TASK_ID`**: Unique identifier for the task (used in the notification URL)
- **`JOB_ITEM_MSG_NOTIF_HOST`**: Base URL for the notification endpoint

```bash
# Example setup
export JOB_ITEM_TASK_ID=monitoring-task-001
export JOB_ITEM_MSG_NOTIF_HOST=http://localhost:8080/msg/notif

# Now use the save command
df -h | ./job_item save
```

#### Behavior
1. **Reads from stdin**: Captures all piped input line by line
2. **Real-time display**: Shows captured content on stdout as it's received
3. **Notification**: Sends the complete captured output to the notification endpoint
4. **Error handling**: Provides clear error messages if environment variables are missing

#### Example Integration
```bash
#!/bin/bash
# monitoring_script.sh

# Set up environment for notifications
export JOB_ITEM_TASK_ID="system-monitoring-$(date +%Y%m%d-%H%M%S)"
export JOB_ITEM_MSG_NOTIF_HOST="http://job-manager:8080/msg/notif"

# Monitor system resources and send notifications
echo "=== System Monitor Report ===" | ./job_item save
uptime | ./job_item save
free -h | ./job_item save
df -h | ./job_item save
```

### REST API Endpoints

#### Local Job Creation
```bash
POST /job/create
Content-Type: application/json

{
  "app_id": "your-app-id",
  "event": "validate_doc",
  "form_body": {
    "document_id": "doc123",
    "validation_rules": ["spelling", "grammar"]
  }
}
```

#### Message Notifications
```bash
POST /msg/notif/:task_id
Content-Type: application/json

{
  "msg": "Processing document validation..."
}
```

## Job Execution Flow

1. **Job Trigger**: Job Manager publishes a message to the broker
2. **Message Reception**: Worker receives message on subscribed channel
3. **File Creation**: Task data is written to `{task_id}.json`
4. **Command Execution**: Configured command runs with templated parameters
5. **Status Reporting**: Worker publishes execution status back to broker
6. **Cleanup**: Temporary files are cleaned up after execution

### Message Format
```json
{
  "task_id": "uuid-v7-task-id",
  "data": {
    "param1": "value1",
    "param2": "value2"
  },
  "action": "execute"
}
```

### Environment Variables for Job Scripts

When a job is executed, the worker automatically provides several environment variables that job scripts can use to interact with the Job Manager and report progress:

#### Available Environment Variables

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `JOB_MANAGER_HOST` | Base URL of the Job Manager server | `http://job-manager:5280` |
| `JOB_MANAGER_RESULT_URL` | Complete endpoint for submitting job results | `http://job-manager:5280/api/worker/job_record/result/abc123-task-id` |
| `JOB_MANAGER_UPLOAD_FILE` | Endpoint for uploading files related to the job | `http://job-manager:5280/api/worker/job_record/file/abc123-task-id` |
| `JOB_ITEM_TASK_ID` | Unique identifier for the current task | `abc123-task-id` |
| `JOB_ITEM_PROJECT_ID` | Project ID for authentication | `7bd0c868-87a0-4c90-8d27-559677763bb6` |
| `JOB_ITEM_PROJECT_KEY` | Secret key for authentication | `your-secret-key` |
| `JOB_ITEM_MSG_NOTIF_HOST` | Endpoint for sending progress notifications | `http://worker:8080/msg/notif/abc123-task-id` |

**Note**: The `JOB_ITEM_TASK_ID` and `JOB_ITEM_MSG_NOTIF_HOST` variables are also used by the `save` command to send captured output as notifications.

#### Using Environment Variables in Job Scripts

**Shell Script Example:**
```bash
#!/bin/bash

# Get task information
echo "Processing task: $JOB_ITEM_TASK_ID"

# Send progress notification
curl -X POST "$JOB_ITEM_MSG_NOTIF_HOST" \
     -H "Content-Type: application/json" \
     -d '{"msg": "Starting document validation..."}'

# Read task data
TASK_DATA=$(cat "${JOB_ITEM_TASK_ID}.json")
echo "Task data: $TASK_DATA"

# Process the task (your custom logic here)
python validate_document.py "$JOB_ITEM_TASK_ID"

# Upload result file
curl -X POST "$JOB_MANAGER_UPLOAD_FILE" \
     -H "Authorization: Bearer $JOB_ITEM_PROJECT_KEY" \
     -F "file=@validation_report.pdf"

# Submit final results
curl -X POST "$JOB_MANAGER_RESULT_URL" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer $JOB_ITEM_PROJECT_KEY" \
     -d '{
       "status": "completed",
       "result": {
         "validation_passed": true,
         "issues_found": 0,
         "report_url": "validation_report.pdf"
       }
     }'

echo "Task completed successfully"
```

**Python Script Example:**
```python
#!/usr/bin/env python3
import os
import json
import requests

# Get environment variables
task_id = os.getenv('JOB_ITEM_TASK_ID')
project_id = os.getenv('JOB_ITEM_PROJECT_ID')
project_key = os.getenv('JOB_ITEM_PROJECT_KEY')
result_url = os.getenv('JOB_MANAGER_RESULT_URL')
upload_url = os.getenv('JOB_MANAGER_UPLOAD_FILE')
notif_url = os.getenv('JOB_ITEM_MSG_NOTIF_HOST')

def send_notification(message):
    """Send progress notification"""
    requests.post(notif_url, json={"msg": message})

def upload_file(file_path):
    """Upload a file to Job Manager"""
    headers = {"Authorization": f"Bearer {project_key}"}
    with open(file_path, 'rb') as f:
        files = {'file': f}
        response = requests.post(upload_url, files=files, headers=headers)
    return response.json()

def submit_result(result_data):
    """Submit final job results"""
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {project_key}"
    }
    response = requests.post(result_url, json=result_data, headers=headers)
    return response.json()

# Main job logic
def main():
    send_notification("Starting email processing...")
    
    # Read task data
    with open(f"{task_id}.json", 'r') as f:
        task_data = json.load(f)
    
    # Process email (your custom logic)
    email_result = process_email(task_data)
    
    # Upload generated files
    if os.path.exists("email_report.html"):
        upload_file("email_report.html")
    
    # Submit results
    result = {
        "status": "completed",
        "emails_sent": email_result['count'],
        "success_rate": email_result['success_rate']
    }
    submit_result(result)
    
    send_notification("Email processing completed")

if __name__ == "__main__":
    main()
```

**Node.js Script Example:**
```javascript
#!/usr/bin/env node
const fs = require('fs');
const axios = require('axios');

// Get environment variables
const taskId = process.env.JOB_ITEM_TASK_ID;
const projectKey = process.env.JOB_ITEM_PROJECT_KEY;
const resultUrl = process.env.JOB_MANAGER_RESULT_URL;
const uploadUrl = process.env.JOB_MANAGER_UPLOAD_FILE;
const notifUrl = process.env.JOB_ITEM_MSG_NOTIF_HOST;

async function sendNotification(message) {
  await axios.post(notifUrl, { msg: message });
}

async function uploadFile(filePath) {
  const FormData = require('form-data');
  const form = new FormData();
  form.append('file', fs.createReadStream(filePath));
  
  return axios.post(uploadUrl, form, {
    headers: {
      ...form.getHeaders(),
      'Authorization': `Bearer ${projectKey}`
    }
  });
}

async function submitResult(resultData) {
  return axios.post(resultUrl, resultData, {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${projectKey}`
    }
  });
}

async function main() {
  try {
    await sendNotification("Starting logo validation...");
    
    // Read task data
    const taskData = JSON.parse(fs.readFileSync(`${taskId}.json`, 'utf8'));
    
    // Process logo (your custom logic)
    const logoResult = await processLogo(taskData);
    
    // Upload processed files
    if (fs.existsSync('processed_logo.png')) {
      await uploadFile('processed_logo.png');
    }
    
    // Submit results
    await submitResult({
      status: 'completed',
      logo_valid: logoResult.isValid,
      dimensions: logoResult.dimensions,
      file_size: logoResult.fileSize
    });
    
    await sendNotification("Logo validation completed");
  } catch (error) {
    await sendNotification(`Error: ${error.message}`);
    process.exit(1);
  }
}

main();
```

#### Benefits of Environment Variables

- **No Configuration Required**: Job scripts automatically receive all necessary connection details
- **Authentication Handled**: Credentials are provided automatically for secure API calls
- **Consistent Interface**: Same environment variables across all job types and scripts
- **Real-time Communication**: Scripts can send progress updates and upload files during execution
- **Error Handling**: Failed jobs can report errors back to the Job Manager
- **File Management**: Easy file upload mechanism for job outputs and artifacts

## Monitoring

### Hardware Information
The worker automatically reports:
- Host ID and hostname
- Operating system and platform
- Kernel version and architecture
- CPU, memory, and disk usage
- Network interface information

### Logging
- Structured logging with error codes
- Process lifecycle events
- Job execution status
- Broker connection status
- Configuration changes

### Health Checks
- Broker connectivity monitoring
- Process health supervision
- Configuration validation
- Resource usage tracking

## Security

### TLS/SSL Configuration
Place certificates in the `tls/` directory:
```
tls/
├── nats/
│   ├── ca.crt
│   ├── client.crt
│   └── client.key
├── rabbitmq/
│   └── server.crt
└── redis/
    ├── ca.crt
    ├── client.crt
    └── client.key
```

### Authentication
- Project-based authentication with Job Manager
- Certificate-based broker authentication
- Secure credential storage

## Deployment

### Systemd Service (Linux)
```ini
[Unit]
Description=Job Item Worker
After=network.target

[Service]
Type=simple
User=job-worker
WorkingDirectory=/opt/job-item
ExecStart=/opt/job-item/job_item --config=/etc/job-item/config.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Docker Deployment
```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o job_item main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/job_item .
COPY config.yaml .
CMD ["./job_item"]
```

### Environment Variables
- `CONFIG_PATH`: Path to configuration file
- `LOG_LEVEL`: Logging level (DEBUG, INFO, WARN, ERROR)
- `WORKER_ID`: Override worker identity ID

## Troubleshooting

### Common Issues

#### Connection Problems
```bash
# Check broker connectivity
telnet broker-host broker-port

# Verify certificates
openssl x509 -in tls/nats/client.crt -text -noout

# Test configuration
./job_item --config=config.yaml --dry-run
```

#### Job Execution Failures
- Check job command syntax in configuration
- Verify file permissions and paths
- Review error codes in logs
- Validate input data format

#### Performance Issues
- Monitor resource usage
- Check broker message queue size
- Review job execution timeouts
- Analyze network latency

### Error Codes
All errors include unique codes for easy debugging:
- `ERR-25230903100`: Download/update errors
- `ERR-303509T3200`: Process execution errors
- `ERR-60350903200`: Child process errors
- `ERR-30350903208`: Hardware info errors

## Development

### Project Structure
```
job_item/
├── main.go                 # Application entry point
├── src/
│   ├── controller/         # HTTP controllers
│   ├── event/             # Event handlers
│   └── helper/            # Utility functions
├── support/               # Core support modules
├── tls/                  # TLS certificates
├── config.yaml           # Configuration file
└── build.sh              # Build script
```

### Dependencies
- **Message Brokers**: NATS, RabbitMQ (AMQP), Redis clients
- **Web Framework**: Gin for REST API
- **Configuration**: YAML parsing and environment variables
- **Process Management**: OS signal handling and process supervision
- **Monitoring**: Hardware information and system metrics
- **Security**: TLS/SSL support and certificate management

### Contributing
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For support and questions:
- Create an issue in the repository
- Check the troubleshooting section
- Review error codes in logs
- Consult the configuration examples

## Version History

- **v0.1.1**: Initial release with basic job execution
- **v3**: Current version with enhanced broker support and monitoring

---

**Note**: This worker is designed to work in conjunction with a Job Manager system. Ensure your Job Manager is properly configured to communicate with worker nodes through the chosen message broker.
