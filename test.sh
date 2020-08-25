#! /bin/bash
go build
telegraf --config ./tests/telegraf.conf 
