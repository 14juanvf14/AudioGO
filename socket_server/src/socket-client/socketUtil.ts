import { io } from "socket.io-client";
import { SOCKET_URL } from "./socket.constants";

const socket = io(SOCKET_URL, {
  autoConnect: true
});

export default socket;