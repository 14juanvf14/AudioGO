import express from 'express';
import cors from 'cors';
import { SocketClientService } from './socket-client/socket.client.service';
import { ENV_CONFIG } from './env';

const app = express();
const PORT = ENV_CONFIG.PORT;

app.use(cors());
app.use(express.json());

const socketClientService = new SocketClientService();


app.listen(PORT, () => {
  console.log(`Server is running on port ${PORT}`);
  console.log(`Socket service initialized and connecting...`);
  console.log(`Socket connection status: ${socketClientService.getConnectionStatus()}`);
});

// Graceful shutdown
process.on('SIGINT', () => {
  console.log('Shutting down server...');
  socketClientService.cleanup();
  process.exit(0);
});

process.on('SIGTERM', () => {
  console.log('Server terminated');
  socketClientService.cleanup();
  process.exit(0);
});
