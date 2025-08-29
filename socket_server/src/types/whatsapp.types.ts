export interface WhatsAppCallSession {
  sdp: string;
  sdp_type?: string;
}

export interface WhatsAppCall {
  id: string;
  event: 'connect' | 'terminate';
  to?: string;
  from?: string;
  name?: string;
  session: WhatsAppCallSession;
}

export interface WhatsAppWebhookData {
  "contacts"?: Array<{
    "profile"?: {
      "name"?: string;
    };
    "wa_id"?: string;
  }>, 
  "calls"?: Array<WhatsAppCall>
}

export type CallStatus = 
  | 'idle'
  | 'mic_error'
  | 'mic_ready'
  | 'incoming'
  | 'negotiating'
  | 'pre_accept'
  | 'accept'
  | 'success'
  | 'error';

export interface CallState {
  isConnected: boolean;
  callId: string;
  status: CallStatus;
  error: string;
  answerSDP: string;
  callTerminating: boolean;
  callTerminated: boolean;
  callTerminateError: string | null;
  incomingCall: WhatsAppCall | null;
  showIncomingModal: boolean;
  micEnabled: boolean;
  remoteSpeaking: boolean;
  rejectingCall: boolean;
  rejectError: string | null;
}