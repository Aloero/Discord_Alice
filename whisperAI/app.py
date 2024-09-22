import whisper
import torch
import numpy as np

import grpc
from concurrent import futures
import stream_pb2
import stream_pb2_grpc

import librosa

from scipy import signal
import scipy.io.wavfile

SAMPLE_RATE = 16000

class StreamService(stream_pb2_grpc.StreamServiceServicer):
    def Chat(self, request, context):
        body_bytes = request.body

        audio = convert_audio(body_bytes, input_sample_rate=48000, output_sample_rate=SAMPLE_RATE)

        text = audio_to_text(audio, target_language="ru")
        print(f"Recognized: {text}")

        return stream_pb2.Response(body=text)

# Конвертация аудио из формата (48 Кгц, int16, стерео) в формат (16 Кгц, int16, моно) для обработки нейросетью
def convert_audio(input_bytes, input_sample_rate=48000, output_sample_rate=16000):
    audio_data = np.frombuffer(input_bytes, dtype=np.int16)
    
    y_resampled = librosa.resample(audio_data.astype(float), orig_sr=input_sample_rate, target_sr=output_sample_rate)
    
    y_resampled_int16 = np.clip(y_resampled, -32768, 32767).astype(np.int16)
    
    return y_resampled_int16

# Обработка аудио нейросетью для преобразования его в текст
def audio_to_text(audio_data, target_language="en"):
    audio_np = np.frombuffer(audio_data, dtype=np.int16).astype(np.float32)
    
    audio_np = audio_np / np.max(np.abs(audio_np))

    audio_tensor = torch.from_numpy(audio_np)
    
    audio_tensor = whisper.pad_or_trim(audio_tensor)

    mel = whisper.log_mel_spectrogram(audio_tensor).to(model.device)
    
    result = model.transcribe(audio_tensor, language=target_language)
    return result['text']

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    stream_pb2_grpc.add_StreamServiceServicer_to_server(StreamService(), server)
    server.add_insecure_port('[::]:50051')
    print("Server is running on port 50051")
    server.start()
    server.wait_for_termination()

if __name__ == '__main__':
    path = "model_base.pt"
    model = whisper.load_model("medium")
    serve()