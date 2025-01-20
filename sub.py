# sub.py
import whisper
import json
import sys
import os
import warnings

# TODO: Play around with the # of words on the screen at a time since it gets long sometimes
def split_segment_into_short_segments(segment, max_words=3):
    """
    Split a segment into multiple shorter segments, each with accurate text and timestamps,
    ending early if punctuation like period (.), comma (,), or other symbols are encountered.
    """
    short_segments = []
    words = segment.get("words", [])
    chunk = []

    for word in words:
        # Strip leading and trailing spaces from the word
        word["word"] = word["word"].strip()

        chunk.append(word)
        if len(chunk) >= max_words or word["word"][-1] in ".,!?":
            # Clean up trailing punctuation if it's the end of the chunk
            if word["word"][-1] in ".,!?":
                word["word"] = word["word"][:-1]

            # Create a new short segment
            start_time = chunk[0]["start"]
            end_time = chunk[-1]["end"]

            # Join words with a single space
            text = " ".join(w["word"] for w in chunk)
            short_segments.append({
                "start": start_time,
                "end": end_time,
                "text": text
            })
            chunk = []  # Reset the chunk

    # Handle any remaining words in the chunk
    if chunk:
        start_time = chunk[0]["start"]
        end_time = chunk[-1]["end"]
        text = " ".join(w["word"] for w in chunk)
        short_segments.append({
            "start": start_time,
            "end": end_time,
            "text": text
        })

    return short_segments

def transcribe_audio(audio_file):
    try:
        # Verify file exists
        if not os.path.exists(audio_file):
            raise FileNotFoundError(f"Audio file not found: {audio_file}")

        # Load the model and transcribe
        model = whisper.load_model("small", device="cuda")  # Use GPU for inference
        result = model.transcribe(audio_file, word_timestamps=True)

        # Split each segment into shorter segments
        all_short_segments = []
        for segment in result["segments"]:
            short_segments = split_segment_into_short_segments(segment, max_words=3)
            all_short_segments.extend(short_segments)

        return all_short_segments
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
        segments = transcribe_audio(audio_file)
        print(json.dumps(segments, indent=2))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)
