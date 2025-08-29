import axios from "axios";
import { ENV_CONFIG } from '../env';


export interface WhatsAppApiResponse {
  success: boolean;
  [key: string]: unknown;
}


class WhatsappWsRestService {
  private endpoint = ENV_CONFIG.WHATSAPP_API_ENDPOINT;

  private makeRequest = async (
    body: any
  ) => {

    console.log("makeRequest", body);
    const response = await axios.post(this.endpoint, body, {
      headers: {
        'Content-Type': 'application/json',
        'ApiKey': ENV_CONFIG.WHATSAPP_API_KEY, 
        'UserId': ENV_CONFIG.WHATSAPP_USER_ID
      }
    });
    return response.data;
  }

  public async preAcceptCall(callId: string, sdp: string): Promise<WhatsAppApiResponse> {
    return this.makeRequest({
      messaging_product: 'whatsapp',
      call_id: callId,
      action: 'pre_accept',
      session: { sdp_type: 'answer', sdp }
    });
  }

  public async acceptCall(callId: string, sdp: string): Promise<WhatsAppApiResponse> {
    return this.makeRequest({
      messaging_product: 'whatsapp',
      call_id: callId,
      action: 'accept',
      session: { sdp_type: 'answer', sdp }
    });
  }

  public async rejectCall(callId: string): Promise<WhatsAppApiResponse> {
    return this.makeRequest({
      messaging_product: 'whatsapp',
      call_id: callId,
      action: 'reject'
    });
  }

  public async terminateCall(callId: string): Promise<WhatsAppApiResponse> {
    return this.makeRequest( {
      messaging_product: 'whatsapp',
      call_id: callId,
      action: 'terminate'
    });
  }
}

export default new WhatsappWsRestService();