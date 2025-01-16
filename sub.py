# sub.py
import whisper
import json
import sys
import os
import warnings

def transcribe_audio(audio_file):
    try:
        # Verify file exists
        if not os.path.exists(audio_file):
            raise FileNotFoundError(f"Audio file not found: {audio_file}")
            
        # Load the model and transcribe
        model = whisper.load_model("base", device="cpu")
        result = model.transcribe(audio_file)
        return result['segments']
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(json.dumps({"error": "Please provide an audio file path as argument"}))
        sys.exit(1)
    warnings.filterwarnings("ignore")

    try:
        audio_file = sys.argv[1]
        print(audio_file +  "!!!!!!!!!!!")
        segments = transcribe_audio(audio_file)
        print(json.dumps(segments))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)