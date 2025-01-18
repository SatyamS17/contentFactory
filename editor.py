from moviepy import VideoFileClip, TextClip, CompositeVideoClip, AudioFileClip, ImageClip, concatenate_videoclips, CompositeAudioClip
import sys
import os

def read_subtitle_file(file_path):
    subtitles = []
    with open(file_path, 'r', encoding='utf-8') as file:
        lines = file.readlines()
        
    i = 0
    while i < len(lines):
        if lines[i].strip().isdigit():  # Subtitle number
            # Time line format: "00,000 --> 00,400"
            times = lines[i + 1].strip().split(' --> ')
            start_time = float(times[0].replace(',', '.'))  # Convert to seconds
            end_time = float(times[1].replace(',', '.'))
            
            # Get text (might be multiple lines)
            text = ''
            i += 2
            while i < len(lines) and lines[i].strip():
                text += lines[i].strip() + ' '
                i += 1
                
            subtitles.append({
                'start': start_time,
                'end': end_time,
                'text': text.strip()
            })
        i += 1
    return subtitles

# Hide outputs to prevent cluttering terminal
original_stdout = sys.stdout
original_stderr = sys.stderr
sys.stdout = open(os.devnull, 'w')
sys.stderr = open(os.devnull, 'w')

# Load audio
bodyAudio = AudioFileClip("audio/text-to-speech/post_body.mp3").with_volume_scaled(1.5)
titleAudio = AudioFileClip("audio/text-to-speech/post_title.mp3").with_volume_scaled(1.5)
backgroundMusic = AudioFileClip("audio/music/music.mp3", ).with_volume_scaled(0.2)

# Load your video
titleClip = (
    VideoFileClip("video/minecraft.mp4")
    .subclipped(10, 10 + titleAudio.duration)
    .without_audio()
)

bodyClip = (
    VideoFileClip("video/minecraft.mp4")
    .subclipped(10 + titleAudio.duration, 10 + titleAudio.duration + bodyAudio.duration)
    .without_audio()
)

# Load and configure your image
title_image_clip = (
    ImageClip("video/reddit.png")
    .with_duration(titleAudio.duration)
    .resized(0.57)
    .with_position((10, 215))
)

# Read body subtitles
body_subtitles = read_subtitle_file("audio/text-to-speech/subtitles.txt")

# Create subtitle clips for body
body_subtitle_clips = []
for sub in body_subtitles:
    txt_clip = (TextClip(
        text=sub['text'],
        font="fonts/Milker.otf",
        font_size=30,
        color='white',
        stroke_color="black",
        stroke_width=3,
        size=(325, None),
        method='caption',
        text_align="center",
    )
    .with_duration(sub['end'] - sub['start'])
    .with_position('center')
    .with_start(sub['start']))
    
    body_subtitle_clips.append(txt_clip)

# Composite video clips
title_video = CompositeVideoClip([titleClip, title_image_clip]).with_duration(titleAudio.duration)
body_video = CompositeVideoClip([bodyClip] + body_subtitle_clips).with_duration(bodyAudio.duration)

# Combine video clips
final_video = concatenate_videoclips([title_video, body_video], method="compose")

# Combine all audio tracks
# loopedBackgroundMusic = backgroundMusic.loop(duration=final_video.duration)
final_audio = CompositeAudioClip([
    backgroundMusic,
    titleAudio.with_start(0),
    bodyAudio.with_start(titleAudio.duration)
])

final_video = final_video.with_audio(final_audio)

# Show loading bar
sys.stdout.close()
sys.stderr.close()
sys.stdout = original_stdout
sys.stderr = original_stderr

# Export
final_video.write_videofile("video/result.mp4", codec="libx264", audio_codec="libmp3lame", threads=12)
