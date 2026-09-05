from taskqueue import TaskQueueWorker

def process_ml_inference(payload):
    """A simulated Python Machine Learning task."""
    text = payload.get("text", "")
    print(f"Running ML inference on: {text}")
    
    # Simulate heavy Python logic (numpy, pandas, pytorch, etc.)
    return {
        "sentiment": "positive",
        "confidence": 0.98,
        "input": text
    }

def main():
    # Create the Python worker server
    worker = TaskQueueWorker(port=5050)
    
    # Register the ML function. 
    # When the Go engine sends a job to /ml_inference, this runs.
    worker.register_handler("ml_inference", process_ml_inference)
    
    # Start listening for jobs
    worker.start()

if __name__ == "__main__":
    main()
