# Contenedor que ejecuta NanoMQ
# Se usa para recibir datos mediante el protocolo MQTT
docker run -d -p 1883:1883 -p 8883:8883 --name nanomq emqx/nanomq:latest

# iniciar un contenedor que ejecuta localmente MiniO
docker run -d -p 9000:9000 -p 9001:9001 --name minio1 -e "MINIO_ACCESS_KEY=miniominio" -e "MINIO_SECRET_KEY=miniominio" minio/minio server /data --console-address ":9001" --address ":9000"
