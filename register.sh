#!/bin/bash

curl -vv \
    --header "Content-Type: application/json" \
    --request POST \
    --data @payload.json \
    http://localhost:30000/register
