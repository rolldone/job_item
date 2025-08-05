import { Hono } from 'hono';
import { createServer } from 'http';
import { writeFileSync, readFileSync, existsSync, unlinkSync } from 'fs';

const app = new Hono();

// Define a simple route
app.get('/', (c) => {
    console.log('Request received');
    return c.text('Hello, World!');
});

// Function to write the current PID to pid.txt
const writePidToFile = () => {
    const pid = process.pid;
    writeFileSync('pid.txt', pid.toString(), 'utf8');
    console.log(`PID ${pid} written to pid.txt`);
};

// Function to introduce a delay
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// Function to check and kill the previous process
const killPreviousProcess = async () => {
    if (existsSync('pid.txt')) {
        try {
            const pid = readFileSync('pid.txt', 'utf8').trim();
            console.log(`Found existing PID: ${pid}`);
            // Kill the process
            process.kill(pid, 'SIGTERM');
            console.log(`Killed process with PID: ${pid}`);
        } catch (err) {
            console.error(`Failed to kill process: ${err.message}`);
        } finally {
            // Remove the pid.txt file
            unlinkSync('pid.txt');
            console.log('Waiting for the port to be released...');
            await delay(5000); // Wait for 2 seconds
        }
    }
};

// Kill the previous process if it exists
// await killPreviousProcess();

// Start the server on a fixed port 7000
const port = 2000;

const server = createServer(async (req, res) => {
    const honoResponse = await app.fetch(req);
    res.writeHead(honoResponse.status, Object.fromEntries(honoResponse.headers));
    res.end(await honoResponse.text());
});

server.listen(port, () => {
    console.log(`Server is running on port ${port}`);
    writePidToFile(); // Write the current PID to pid.txt
    // Set up graceful shutdown
    const shutdown = () => {
        console.log('Shutting down server...');
        server.close(() => {
            console.log('Server closed');
            if (existsSync('pid.txt')) {
                unlinkSync('pid.txt');
            }
            // process.exit(1);
        });
        
        // Force exit after timeout
        setTimeout(() => {
            console.log('Force exit after timeout');
            process.exit(0);
        }, 10000); // 10 seconds
    };
    shutdown()
});

server.on('error', (err) => {
    if (err.code === 'EADDRINUSE') {
        console.error(`Port ${port} is already in use. Exiting...`);
        process.exit(1);
    } else {
        console.error(`Server error: ${err}`);
        process.exit(1);
    }
});

