// build-payload-and-post.js
import zlib from "zlib";
import ServerRTC from "../serverRTC.interface.d";
import { ENV_CONFIG } from "../../../env";

const url = ENV_CONFIG.GO_SERVER_SDP_URL;


function decodeBase64(str: string) {
  let buf = Buffer.from(str, "base64");
  buf = Buffer.from(zlib.gunzipSync(buf));              // descomprime gzip
  return JSON.parse(buf.toString("utf8")); // parsea JSON
}

export const decodeSignal = (payload: string) => {
  const [answerEncoded, _] = payload.split(";");
  const answer = decodeBase64(answerEncoded);

  console.log("Answer SDP:\n", answer.sdp);
  return answer.sdp;
}

function signalEncode(obj: any) {
  const compress = true;
  let buf = Buffer.from(JSON.stringify(obj), "utf8");
  if (compress) buf = Buffer.from(zlib.gzipSync(buf));
  return buf.toString("base64");
}


export const encodeSignal = async (sdpOffer: string) => {

  const sdp = sdpOffer;
  const offerEncoded = signalEncode({ type: "offer", sdp });
  const candidatesEncoded = signalEncode([]); // sin trickle ICE aparte

  const payload = `${offerEncoded};${candidatesEncoded}`;
  console.log(`Payload to ${url}: `, payload, "\n");

  // Enviar al server Go (opcional):
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "text/plain" },
      body: payload,
    });
    const text = await res.text();

    console.log("Respuesta del server Go:", text);
    if (!res.ok) throw new Error(`HTTP ${res.status}: ${text}`);

    return text;
  } catch (e: any) {
    console.error("Fallo POST /sdp:", e.message);
  }
};

class GoServerAnswer implements ServerRTC {
  public async generateAnswer(sdpOffer: string): Promise<string> {
    const answerServerGo = await encodeSignal(sdpOffer);
    if (!answerServerGo) throw new Error("No answerServerGo from server Go");
  
    const answerSDP = decodeSignal(answerServerGo);
    console.log("Answer SDP:", answerSDP);
  
    return answerSDP;
  }
}

export default GoServerAnswer;