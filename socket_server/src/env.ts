// Environment configuration
export const ENV_CONFIG = {
  // Server configuration
  PORT: process.env.PORT || 3000,
  
  // Socket configuration
  SOCKET_URL: process.env.SOCKET_URL || "https://inbound-socket.aldeamo.com",
  USER_INITIATED_ROOM: process.env.USER_INITIATED_ROOM || "new-call:a7104d2f-75a5-400d-9770-3a380efaaf3a",
  
  // WhatsApp API configuration
  /*
  ESTO ES SANJI
  WHATSAPP_API_ENDPOINT: process.env.WHATSAPP_API_ENDPOINT || "https://apitellitwhatsapp.aldeamo.net/v3/call/573053673966",
  WHATSAPP_API_KEY: process.env.WHATSAPP_API_KEY || "api_key",
  WHATSAPP_USER_ID: process.env.WHATSAPP_USER_ID || "9826",
  */

  WHATSAPP_API_ENDPOINT: process.env.WHATSAPP_API_ENDPOINT || "https://apitellitwa.aldeamo.com/v3/call/573053673966",
  WHATSAPP_API_KEY: process.env.WHATSAPP_API_KEY || "OTFhNjU5MTMtMjhmMy00Y2IyLTgxZDUtYjRhOWY4OTQ5NDEw",
  WHATSAPP_USER_ID: process.env.WHATSAPP_USER_ID || "15056",

  // WebRTC Go Server configuration
  GO_SERVER_SDP_URL: process.env.GO_SERVER_SDP_URL || "http://localhost:8080/sdp" || "https://blbzlwgz-8080.use.devtunnels.ms/sdp",
} as const;
