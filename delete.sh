#!/bin/bash

source /etc/bashrc

if [ `kubectl get app 2>/dev/null|grep app-sample |wc -l` -ne 0 ];then
	kubectl delete app app-sample
fi

if [ `kubectl get deploy 2>/dev/null|grep app-sample |wc -l` -ne 0 ];then
        kubectl delete deploy app-sample
fi

if [ `kubectl get svc 2>/dev/null|grep app-sample |wc -l` -ne 0 ];then
        kubectl delete svc app-sample
fi

if [ `kubectl get ing 2>/dev/null|grep app-sample |wc -l` -ne 0 ];then
        kubectl delete ing app-sample
fi
