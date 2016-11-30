FROM centurylink/ca-certs
COPY app /
ENTRYPOINT ["/app"]
