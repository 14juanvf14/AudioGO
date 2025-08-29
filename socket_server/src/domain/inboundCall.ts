import whatsappWsRestService from "../services/whatsapp-ws-rest.service";
import { WhatsAppCall, WhatsAppWebhookData } from "../types/whatsapp.types";
import { isPhoneNumberAllowed } from "../utils/phoneNumbersAllowed";
import GoServerAnswer from "./serverRTC/go/goServerAnswer";
import ServerRTC from "./serverRTC/serverRTC.interface";

export const handleIncomingCall = async (message: WhatsAppWebhookData): Promise<void>  => {
  const call = message?.calls?.[0];
  
  if (!call) throw new Error("No call data found in message");
  if (!isPhoneNumberAllowed(call.from ?? "")) throw new Error(`Call rejected from unauthorized number: ${call.from}`);
  
  if (call.event == 'terminate') return eventTerminateCall(call);
  if (call.event == 'connect') return eventConnectCall(call);

  console.log("Call event not supported: ", call?.event);
}


const eventTerminateCall = (call: WhatsAppCall) => {
  console.log(`Call terminated: ${call.id}`);
  return;
}


const eventConnectCall = async (call: WhatsAppCall) => {
  try {
    console.log(`Accepting call from allowed number: ${call.from}`);
    console.log('SDP OFFER: ', call.session.sdp);
  
    const strategyAnswer: ServerRTC = new GoServerAnswer();
    const answerSDP = await strategyAnswer.generateAnswer(call.session.sdp);

    const preAcceptCall = await whatsappWsRestService.preAcceptCall(call.id, answerSDP);
    console.log("preAcceptCall:", preAcceptCall);

    const acceptCall = await whatsappWsRestService.acceptCall(call.id, answerSDP);
    console.log("acceptCall:", acceptCall);
  } catch (error) {
    console.error("Error accepting call:", error);
  }
}
