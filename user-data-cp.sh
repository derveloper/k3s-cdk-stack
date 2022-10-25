#!/bin/bash -xe

exec > >(tee /var/log/user-data.log|logger -t user-data -s 2>/dev/console) 2>&1
  yum -y update
  public_ip=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4)
  curl -sfL https://get.k3s.io | K3S_TOKEN=%s sh - --node-external-ip "$public_ip"
  reboot