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

// display env
console.log('Environment Variables:');
for (const [key, value] of Object.entries(process.env)) {
    console.log(`${key}: ${value}`);
}



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
            process.exit(1);
        }, 10000); // 10 seconds
    };
    // shutdown()
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

const postData = async () => {
    const jobRequest = {
        app_id: process.env.JOB_ITEM_APP_ID, // Replace with actual AppId
        event: "print_doc.pdf", // Replace with actual Event
        form_body: { key: 'value' } // Replace with actual FormBody data
    };

    const url = process.env.JOB_ITEM_CREATE_URL;
    if (!url) {
        console.error('Environment variable JOB_ITEM_CREATE_URL is not set');
        return;
    }

    try {
        const response = await fetch(url, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(jobRequest),
        });

        if (!response.ok) {
            console.error(`Failed to post data: ${response.statusText}`);
        } else {
            const responseData = await response.json();
            console.log('Response:', responseData);
        }
    } catch (error) {
        console.error('Error posting data:', error);
    }
};

setTimeout(() => {
    postData();
    setTimeout(() => {
        throw new Error("Ini error yang dipaksa");
    }, 20000); // Wait for 20 seconds before next action
}, 5000);

