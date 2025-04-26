FROM debian:12

WORKDIR /app
ADD tuss.elf /app/tuss
RUN chmod a+x /app/tuss
# VOLUME [ "/app/config" ]
EXPOSE 8080

CMD ["/app/tuss"]
# docker build -t regsvr:latest .
# docker run -d --name regsvr -v /path/to/config:/app/config -p 8001:8001 -p 18001:18001 regsvr:latest
# docker run -d --name regsvr -v /path/to/config:/app/config -p 8001:8001 -p 18001:18001 --restart always regsvr:latest
