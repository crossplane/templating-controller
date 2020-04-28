FROM BASEIMAGE
RUN apk --no-cache add ca-certificates bash

ARG ARCH
ARG TINI_VERSION

ADD templating-controller /usr/local/bin/
EXPOSE 8080
USER 1001
ENTRYPOINT ["templating-controller"]