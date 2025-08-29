export default abstract class ServerRTC {
  public abstract generateAnswer(sdpOffer: string): Promise<string>;
}