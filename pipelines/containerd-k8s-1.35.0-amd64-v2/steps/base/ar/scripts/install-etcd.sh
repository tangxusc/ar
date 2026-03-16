#!/bin/bash
set -e

sudo tar -xf /tmp/ar/tar/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz -C /tmp/ 
sudo mv /tmp/etcd-${ETCD_VERSION}-linux-amd64/etcd /usr/local/bin/ 
sudo mv /tmp/etcd-${ETCD_VERSION}-linux-amd64/etcdctl /usr/local/bin/
sudo chmod +x /usr/local/bin/etcd /usr/local/bin/etcdctl
sudo rm -rf /tmp/etcd-${ETCD_VERSION}-linux-amd64
sudo etcd --version


sudo cp /tmp/ar/service/etcd.service /usr/lib/systemd/system/etcd.service
sudo systemctl daemon-reload
sudo systemctl enable etcd
sudo systemctl start etcd
sudo systemctl status etcd