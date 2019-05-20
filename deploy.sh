#!/bin/sh

docker-compose build
docker save triebwerk | bzip2 | pv | ssh root@142.93.104.75 'bunzip2 | docker load'
ssh root@142.93.104.75 'docker-compose up -d'

