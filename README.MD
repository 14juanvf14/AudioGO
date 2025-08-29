#Primer paso: encender servidor socket que escucha las llamadas entrantes
cd socket_server && npm i && npm run dev


#Segundo paso: encender servidor SDU (servidor en go)
go run main.go