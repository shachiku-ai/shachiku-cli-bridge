#!/bin/sh

# This is a mock CLI used for testing
# It outputs the received arguments
echo "Received args: $@"

# And simulating streaming and colored outputs
sleep 0.1
printf "\033[31mColored Text\033[0m\n"
sleep 0.1
echo "Stream testing done."
