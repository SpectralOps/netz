FROM golang:1.14

RUN apt-get update
RUN apt-get install -y git build-essential curl wget jq libpcap-dev gettext-base

RUN git clone https://github.com/robertdavidgraham/masscan /opt/masscan
WORKDIR /opt/masscan
RUN sed -i 's/\\r\"/\\n\\r\"/g' src/main-status.c
RUN make -j
RUN cp /opt/masscan/bin/masscan /usr/local/bin

RUN go get github.com/zmap/zgrab2 && \ 
cd $GOPATH/src/github.com/zmap/zgrab2 && \
make && ln -s $GOPATH/src/github.com/zmap/zgrab2/cmd/zgrab2/zgrab2 /usr/local/bin/zgrab2

WORKDIR /opt

ADD discover.sh /opt
ADD zgrab2-template.ini /opt
ADD massconfigure.go /opt

CMD ["/opt/discover.sh"]
