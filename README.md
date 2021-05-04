[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT) [![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg?style=flat-square)](http://makeapullrequest.com)


# netz :globe_with_meridians::eagle:

The purpose of this project is to discover an internet-wide misconfiguration of network components like web-servers/databases/cache-services and more.
The basic use-case for such misconfiguration - a service that is publicly exposed to the world without a credentials `¯\_(ツ)_/¯`     

You probably familiar with tools like [Shodan](https://www.shodan.io/), [Censys](https://censys.io/), [ZoomEye](https://www.zoomeye.org/) to query such wide internet components,  
but here we are going to do it in a fun way :: by hands :D  

The tools we are going to use are [masscan](https://github.com/robertdavidgraham/masscan), and [zgrab2](https://github.com/zmap/zgrab2) from [ZMap](https://zmap.io/) project. For the first phase of port scanning, we will use [masscan](https://github.com/robertdavidgraham/masscan), then for the second phase, we will run [zgrab2](https://github.com/zmap/zgrab2) to check applicative access for those ports.

[ZMap](https://github.com/zmap/zmap) is also internet-wide scanner, so why [masscan](https://github.com/robertdavidgraham/masscan) and not [ZMap](https://github.com/zmap/zmap)..?
because we want to go wild and use kernel module [PF_RING ZC (Zero Copy)](https://www.ntop.org/products/packet-capture/pf_ring/pf_ring-zc-zero-copy/) to get blazing fast packets-per-second to scan the entire internet in minutes, 
and [ZMap](https://github.com/zmap/zmap) basically does support it in the past, but now [ZMap](https://github.com/zmap/zmap) doesn't compatible with the latest [PF_RING ZC (Zero Copy)](https://www.ntop.org/products/packet-capture/pf_ring/pf_ring-zc-zero-copy/).

Note that [PF_RING ZC (Zero Copy)](https://www.ntop.org/products/packet-capture/pf_ring/pf_ring-zc-zero-copy/) requires a license per MAC/NIC (you can run 5 minutes in demo before it will kill the flow), and you need a special NIC from Intel (don't worry, the public cloud has such) so you can go without this module, and pay on time to wait for results.

There are few options to run this project:

1. Use netz cloud runner tool - this tool automate the full pipeline, including infrastructure on top of AWS
2. Run by yourself using docker 
3. For [PF_RING ZC (Zero Copy)](https://www.ntop.org/products/packet-capture/pf_ring/pf_ring-zc-zero-copy/) run by yourself the infrastructure and using [pf_ring setup](pf_ring/configure_pf_ring.sh)

If you want to read more about it, you can found it here: [Scan the whole internet while drinking coffee](https://cmpxchg16.medium.com/scan-the-whole-internet-while-drinking-coffee-9c4085539594)

## TL;DR

In [discover.sh](docker/discover.sh) you will find a test for [Elasticsearch](https://www.elastic.co/).  
The flow is: 
* run [masscan](https://github.com/robertdavidgraham/masscan) on the entire internet for port 9200 ([Elasticsearch](https://www.elastic.co/) port)
* pipe ip list from step 1 into [zgrab2](https://github.com/zmap/zgrab2) (you can change with `ZGRAB2_ENDPOINT` environment variable for any [Elasticsearch](https://www.elastic.co/) API Endpoint, for instance: `/_cat/indices`  
* extract with [jq](https://stedolan.github.io/jq/) just those ip's that return HTTP 200 OK and include `lucene_version`  

This flow result is ips' that has internet access to [Elasticsearch](https://www.elastic.co/) without credentials.    

This test flow demonstrates [Elasticsearch](https://www.elastic.co/) scan. You can run such scans on any port (service port) you wish and on any supported protocol by [zgrab2 modules](https://github.com/zmap/zgrab2/tree/master/modules). Environment variables can modify more control:      
`PORT_TO_SCAN`  
`SUBNET_TO_SCAN`  
`ZGRAB2_ENDPOINT`

In case you wish to add a missing protocol, you can extend [zgrab2](https://github.com/zmap/zgrab2) by [adding new protocols](https://github.com/zmap/zgrab2#adding-new-protocols.)  


We will go through a setup to be faster and faster (decreasing the time to wait).

## Let's Go :rocket:
## 1. netz cloud runner tool
This is the easiest option as it automates everything in AWS on top of Elastic Container Service (ECS).  
What it does:  

* Create IAM role for the pipeline
* Put IAM Policy
* Create Instance Profile
* Associate IAM role to Instance Profile
* Create Temporary ECS Cluster
* Create EC2 instance (instance type based on user input `--instance-type`)
* Create a number of Network Interfaces (number based on user input `--number-of-nic`)
* Create Public Elastic IP (number based on user input `--number-of-nic`)
* Associate Elastic IP with Network Interface (for each user input `--number-of-nic`)
* Run ECS task with the scanning pipeline
* Create CloudWatch log group and stream the pipeline docker output into the user terminal
* Destroying all AWS resources
* Done

## How to run  
Configure AWS credentials, you can do it by `~/.aws/credentials`,  
or by settings environment variables:  
`AWS_REGION`  
`AWS_ACCESS_KEY_ID`  
`AWS_SECRET_ACCESS_KEY`  

[Install Golang 1.14 +](https://golang.org/dl/)  

```
$ go build github.com/SpectralOps/netz
$ netz
NAME:
   netz - netz cloud runner

USAGE:
   netz [options]

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug                        Show debugging information (default: false)
   --file value                   Task definition file in JSON or YAML
   --cluster value                ECS cluster name (default: "netz")
   --log-group value              Cloudwatch Log Group Name to write logs to (default: "netz-runner")
   --security-group value         Security groups to launch task. Can be specified multiple times
   --subnet value                 Subnet to launch task.
   --region value                 AWS Region
   --number-of-nic value          Number of network interfaces to create and attach to instance. (default: 0)
   --instance-type value          Instance type.
   --instance-key-name value      Instance key name to for ssh.
   --role-name value              Role name for netz. (default: "netzRole")
   --role-policy-name value       Role policy name for netz. (default: "netzPolicy")
   --instance-profile-name value  Instance profile name to attach to instance. (default: "netzInstanceProfile")
   --task-timeout value           Task timeout (in minutes), stop everything after that. (default: 120)
   --skip-destroy                 Skip destroy of cloud resources when done. (default: false)
   --help, -h                     show help (default: false)
Required flags "file, security-group, subnet, region, number-of-nic, instance-type, instance-key-name"
```

### Example
```
$ netz --file taskdefinition.json --security-group sg-XXXXXXXXXXXXXXXXXX --subnet subnet-XXXXXXXX --region us-west-1 --debug --number-of-nic 5 --instance-type c4.8xlarge --instance-key-name XXXXXXXXX
```

:warning:    
**Because masscan meltdown the network, SSH mostly will not be available, also CloudWatch logs will be deferred, so the tailed logs in user terminal will take some time.**  

Note that [taskdefinition.json](taskdefinition.json) is related to running with the automated way with AWS ECS.  
In that file, you will be able to change the subnet & port to scan, also the application endpoint.  
In this file, you can also control the CPU & RAM you allocate to the task. This test assumed c4.8xlarge, so the config is `60 x cpu` and `36 GB RAM`.  

### Result
On AWS with c4.8xlarge with 6 x NIC ~ 2.9M ~ 3.5M PPS => took 25 minutes  

## 2. Run by yourself using docker

### 2.1 Basic
#### Run with Docker on basic computer/NIC
##### Steps
```bash
$ git clone https://github.com/SpectralOps/netz
$ cd netz
$ docker build -t netz .
$ docker run -e PORT_TO_SCAN='80' -e SUBNET_TO_SCAN='216.239.38.21/32' -e ZGRAB2_ENDPOINT='/' -e TASK_DEFINITION='docker' -v /tmp/:/opt/out --network=host -it netz
```
:warning:    
**The time to scrape the entire internet with simple hardware and simple internet backbone could take days**


### 3. Faster :zap:
#### Run with Docker on Cloud with one 10gbps NIC

Run instance with one 10gbps NIC (e.g. in AWS c4.8xlarge [already configured with])  

Steps are the same as [2.1 Basic](https://github.com/SpectralOps/netz#21-basic).

### Result
On AWS with c4.8xlarge ~ 700k ~ 950k PPS => took 2.5 hours.

### 4. Faster++ :zap::dizzy:
#### Run with Docker on Cloud with multiple 10gbps NIC (e.g. in AWS c4.8xlarge 10gbps NIC )
* Run in AWS c4.8xlarge Ubuntu 18.04 and connect multiple NIC ([ENI's](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-eni.html))
* For each NIC you need to configure the OS to see those new NIC's.

Edit the netplan file: 
`vim /etc/netplan/50-cloud-init.yaml`

Now it has one NIC:

```yaml
network:
    version: 2
    ethernets:
        ens3:
            dhcp4: true
            match:
                macaddress: 06:XX:XX:XX:XX:XX
            set-name: ens3
```

You need to add the second, the third and so on...
```yaml
network:
    version: 2
    ethernets:
        ens3:
            dhcp4: true
            match:
                macaddress: 03:XX:XX:XX:XX:XX
            set-name: ens3
        ens4:
            dhcp4: true
            match:
                macaddress: 04:XX:XX:XX:XX:XX
            set-name: ens4
        ens5:
            dhcp4: true
            match:
                macaddress: 05:XX:XX:XX:XX:XX
            set-name: ens5
        ens6:
            dhcp4: true
            match:
                macaddress: 06:XX:XX:XX:XX:XX
            set-name: ens6
        ens7:
            dhcp4: true
            match:
                macaddress: 07:XX:XX:XX:XX:XX
            set-name: ens7
```

Apply network configuration: `sudo netplan --debug apply`  

Steps are the same as [2.1 Basic](https://github.com/SpectralOps/netz#21-basic).

Note that now with multiple NICs, the [masscan](https://github.com/robertdavidgraham/masscan) configuration that will be created in `docker run` will contain all NICs:  


e.g masscan.conf:  

```bash
adapter[0] = ens3
router-mac[0] = 06:XX:XX:XX:XX:XX
adapter-ip[0] = 172.31.8.167
adapter-mac[0] = 06:YY:YY:YY:YY:YY
adapter[1] = ens4
router-mac[1] = 06:XX:XX:XX:XX:XX
adapter-ip[1] = 172.31.8.76
adapter-mac[1] = 06:YY:YY:YY:YY:YY
adapter[2] = ens5
router-mac[2] = 06:XX:XX:XX:XX:XX
adapter-ip[2] = 172.31.1.233
adapter-mac[2] = 06:YY:YY:YY:YY:YY
```

### Result
On AWS with c4.8xlarge with 6 x NIC ~ 2.9M ~ 3.5M PPS => took 35 minutes


### 5. Faster++++ :zap::dizzy::tornado:
#### Run on Cloud with 10gbps NIC with [PF_RING ZC (Zero Copy)](https://www.ntop.org/products/packet-capture/pf_ring/pf_ring-zc-zero-copy/)
In case you want to scrape the internet in a few minutes with [PF_RING ZC (Zero Copy)](https://www.ntop.org/products/packet-capture/pf_ring/pf_ring-zc-zero-copy/), you will need to run a machine that supports the kernel device drivers and a machine that has 10gbps NIC.  

Notes:
* Because [PF_RING ZC (Zero Copy)](https://www.ntop.org/products/packet-capture/pf_ring/pf_ring-zc-zero-copy/) bypasses the TCP stack, so in case you have just one NIC ens3 and you will open it with **zc**:enc3, you will lose SSH access. If you still want SSH access, you will need another NIC, e.g. ens4, then open ens4 with **zc**, so it will be **zc**:ens4, so ens3 will continue as management NIC for SSH.
* If you run a machine with 1gbps NIC, it will still be fast, but it will take **x10** more time you could `¯\_(ツ)_/¯`
* You don't have to run such a machine like c4.8xlarge, you can run each machine that supports the ixgbevf  
from: [enhanced networking with the Intel 82599 VF interface](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/sriov-networking.html)


#### Steps

```bash
$ git clone https://github.com/SpectralOps/netz
$ cd netz
```

Edit masscan.conf -- **important** look that now the adapter prefix is **zc:**:

```bash
adapter[0] = zc:ens4
router-mac[0] = 06:XX:XX:XX:XX:XX
adapter-ip[0] = 172.31.8.167
adapter-mac[0] = 06:YY:YY:YY:YY:YY
```

The `adapter-ip` and `adapter-mac` you can get from the command: `ifconfig`  
The `adapter-mac` you can get from the command: `arp -a`  

Run [configure_pf_ring.sh](configure_pf_ring.sh)  

**Before** the kernel module kicked in - this should be the state:

```bash
$ sudo pf_ringcfg --list-interfaces
Name: ens3                 Driver: ixgbevf    [Supported by ZC]
Name: docker0              Driver: bridge
Name: ens6                 Driver: ixgbevf    [Supported by ZC]
Name: ens5                 Driver: ixgbevf    [Supported by ZC]
Name: ens4                 Driver: ixgbevf    [Supported by ZC]
```

**After** the kernel module kicked in - this should be the state:

```bash
$ sudo pf_ringcfg --list-interfaces
Name: ens3                 Driver: ixgbevf    [Running ZC]
Name: docker0              Driver: bridge
Name: ens6                 Driver: ixgbevf    [Running ZC]
Name: ens5                 Driver: ixgbevf    [Running ZC]
Name: ens4                 Driver: ixgbevf    [Running ZC]
```

### Run scan:

```bash
PORT_TO_SCAN='9200' SUBNET_TO_SCAN='0.0.0.0/0' ZGRAB2_ENDPOINT='/' TASK_DEFINITION='docker' bash -x discover.sh
```


### Result
On AWS with c4.8xlarge with 4 x NIC ~ 10.5M ~ 12M PPS => took 10 minutes


# Disclaimer
Our main drive in life is to make the world a better and safer place. If you would like to use this information to harm someone, you are doing the opposite, and at your own risk.    


# Copyright

Copyright (c) 2020 [Uri Shamay](http://cmpxchg16.me) [@cmpxchg16](http://twitter.com/cmpxchg16). See [LICENSE](LICENSE.txt) for further details.
