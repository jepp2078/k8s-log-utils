FROM scratch
MAINTAINER Jeppe Johansen <jeppe.johansen@coop.dk>

ADD bin/linux/dmz-tainter dmz-tainter

ENTRYPOINT ["/dmz-tainter"]