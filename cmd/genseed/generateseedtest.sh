#!/usr/bin/env bash
# ./genseed creates a seed in either mnemonic or its hexadecimal form. 
# by default a hexidecimal seed is created 
# form of the seed can be changed by setting --mne=true 
# size of the seed can be changed by setting --size=<size> 

# Test 1: generateseed with no options
SEED=$(./genseed)
if [[ ${#SEED} -gt 64 || ${#SEED} -lt 16 ]]; then
    echo "test 1 failed" 
    exit 1
fi 

# Test 2: generateseed with option --mne set as true, and --size as 64
PARAMS=$(echo \
    "--mne=true" \
    "--size=64"
)

SEED2=$(./genseed $PARAMS)
if [[ $(echo ${#SEED2} | wc -c) -gt 500 ]]; then
    echo "test 2 failed"
    exit 1
fi
