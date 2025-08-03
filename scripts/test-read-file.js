import { fileURLToPath } from 'url';
import path from 'path';
import fs from 'fs';
import fetch from 'node-fetch';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const filePath = path.join(__dirname, 'test-story.txt');
const CHUNK_SIZE = 100; // Number of characters to read at a time

fs.open(filePath, 'r', (err, fd) => {
    if (err) {
        console.error('Error opening file:', err);
        return;
    }

    const buffer = Buffer.alloc(CHUNK_SIZE);
    let position = 0;

    async function postChunk(chunk) {
        const postData = JSON.stringify({ msg: chunk });

        const url = process.env.JOB_ITEM_MSG_NOTIF_HOST || 'http://localhost:3000/msg/notif';

        try {
            const response = await fetch(url, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: postData
            });

            console.log(`STATUS: ${response.status}`);
            const responseBody = await response.text();
            console.log(`BODY: ${responseBody}`);
        } catch (error) {
            console.error(`Problem with request: ${error.message}`);
        }
    }

    function readNextChunk() {
        fs.read(fd, buffer, 0, CHUNK_SIZE, position, (err, bytesRead) => {
            if (err) {
                console.error('Error reading file:', err);
                fs.close(fd, () => {});
                return;
            }

            if (bytesRead > 0) {
                const chunk = buffer.toString('utf8', 0, bytesRead);
                console.log(chunk);
                postChunk(chunk); // Send the chunk as a POST request
                position += bytesRead;
                setTimeout(readNextChunk, 300); // Add a 300ms delay before reading the next chunk
            } else {
                fs.close(fd, () => {
                    console.log('Finished reading file.');
                });
            }
        });
    }

    readNextChunk();
});