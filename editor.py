from moviepy import VideoFileClip, TextClip, CompositeVideoClip, AudioFileClip

clip = (
    VideoFileClip("minecraft.mp4")
    .subclipped(10, 20)
    .with_volume_scaled(2)  
)

# Generate a text clip
txt_clip = TextClip(
    text="what if the text is very long?????",
    font="Arial.ttf",
    font_size=30,
    color='white',
).with_duration(clip.duration).with_position('center')

# Load an external audio file (optional) or use the existing clip audio
audio = AudioFileClip("text-to-speech\post_1.mp3").with_duration(5)

# Set the audio to the video clip
final_video = CompositeVideoClip([clip, txt_clip]).with_audio(audio).with_volume_scaled(2)

# Export the final video
final_video.write_videofile("result.mp4", codec="libx264", audio_codec="libmp3lame")