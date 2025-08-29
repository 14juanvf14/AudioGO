import socket from './socketUtil';
import { WhatsAppWebhookData } from '../types/whatsapp.types';
import { USER_INITIATED_EVENT, USER_INITIATED_ROOM } from './socket.constants';
import { handleIncomingCall } from '../domain/inboundCall';

export class SocketClientService {
  private isConnected = false;

  constructor() {
    this.initializeSocketListeners();
  }


  private initializeSocketListeners(): void {
    socket.on("connect", () => {
      console.log("Connected to socket");
      this.isConnected = true;
      this.joinRoom();
    });

    socket.on("disconnect", () => {
      console.log("Disconnected from socket");
      this.isConnected = false;
    });

    socket.on("connect_error", (error) => console.error("Socket connection error:", error));
  }


  private joinRoom(): void {
    if (this.isConnected) {

      socket.on(USER_INITIATED_EVENT, (message: WhatsAppWebhookData) => {
        console.log("SOCKET EVENT: ", JSON.stringify(message));
        try {
          handleIncomingCall(message);
        } catch (error) {
          console.error("Error handling incoming call:", error);
        }
      });

      socket.emit('joinRoom', USER_INITIATED_ROOM);
      console.log(`Joined room: ${USER_INITIATED_ROOM} ${USER_INITIATED_EVENT}`);
    }
  }



  public cleanup(): void {
    socket.off("connect");
    socket.off("disconnect");
    socket.off(USER_INITIATED_EVENT);
    socket.off("connect_error");
  }

  public getConnectionStatus(): boolean {
    return this.isConnected;
  }

  public reconnect(): void {
    if (!this.isConnected) {
      socket.connect();
    }
  }
}

