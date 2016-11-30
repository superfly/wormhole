FROM centurylink/ca-certs
ADD app /
ENTRYPOINT ["/app"]
