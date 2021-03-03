#!/bin/sh
apt-get update
apt-get -y -q install wget lsb-release gnupg
wget -q http://apt.ntop.org/18.04/all/apt-ntop.deb && sudo dpkg -i apt-ntop.deb
apt-get clean all
apt-get update
apt-get -y install pfring
echo Y | sudo pf_ringcfg --configure-driver ixgbevf --rss-queues 2
sudo touch /etc/pf_ring/forcestart
sudo systemctl restart pf_ring
sudo cp /usr/local/lib/libpfring.* /usr/lib/
