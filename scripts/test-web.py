import time
import os
from flask import Flask

app = Flask(__name__)


@app.route('/')
def home():
    return "Web server is running on port 4500!"

if __name__ == '__main__':
    try:
        # while True:
        #     print("Loop running with delay", flush=True)
        #     time.sleep(3)
        print("Flask server PID:", os.getpid(), "Port :: ",4500, flush=True)
        app.run(host='0.0.0.0', port=4500)  # Start Flask web server on port 4500
    except KeyboardInterrupt:
        print("Exiting Flask web server...", flush=True)

