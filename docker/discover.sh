#!/bin/sh
go run /opt/massconfigure.go > /opt/masscan.conf

envsubst < zgrab2-template.ini > zgrab2.ini

PORT_TO_SCAN=`echo $PORT_TO_SCAN`
SUBNET_TO_SCAN=`echo $SUBNET_TO_SCAN`
TASK_DEFINITION=`echo $TASK_DEFINITION`

echo masscan config file:
echo
cat /opt/masscan.conf

echo zgrab2 config file:
echo
cat zgrab2.ini

# wait few seconds before start while network card going to meltdown
sleep 5

OUT=/opt/out/masscan-$TASK_DEFINITION.out

masscan -p$PORT_TO_SCAN $SUBNET_TO_SCAN --exclude 255.255.255.255 --rate 10000000 -c /opt/masscan.conf | tee $OUT 2>&1
echo masscan ips:
echo
cat $OUT | awk '{print $6}'
echo zgrab2 ips:
echo
cat $OUT | awk '{print $6}' | zgrab2 multiple -c zgrab2.ini | jq -r '. | select(.data.http.result.response.body != null) | select(.data.http.status == "success") | .ip' | tee /opt/out/zgrab2-$TASK_DEFINITION.out
