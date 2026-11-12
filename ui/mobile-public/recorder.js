// The microphone's samples, handed to the page as they arrive (src/mobile/voice.ts).
class GwenRecorder extends AudioWorkletProcessor {
  process(inputs) {
    const channel = inputs[0] && inputs[0][0];
    if (channel && channel.length) this.port.postMessage(channel.slice(0));
    return true;
  }
}
registerProcessor("gwen-recorder", GwenRecorder);
