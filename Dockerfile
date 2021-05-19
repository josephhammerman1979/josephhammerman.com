FROM alpine:3.10

ADD dist/bin/linux/amd64/squaremeet /bin/squaremeet

CMD ["/bin/squaremeet"]
