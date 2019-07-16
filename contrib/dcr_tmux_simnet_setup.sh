#!/usr/bin/env bash
#
# Copyright (c) 2020 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# Tmux script to create 2 dcrd nodes (named dcrd1 and dcrd2) connected in series
# along with 2 wallets (named wallet1 and wallet2) configured such that wallet1
# is connected via JSON-RPC to dcrd1 and, likewise, wallet2 to dcrd2.
#
# Both wallet1 and wallet2 use the same seed, however, wallet1 is configured to
# automatically buy tickets and vote, while wallet2 is only configured to vote.
#
# The primary dcrd node (dcrd1) is configured as the primary mining node.
#
# Network layout:
# dcrd1 (p2p: localhost:19555) <-- dcrd2 (p2p: localhost:19565)
#
# RPC layout:
# dcrd1 (JSON-RPC: localhost:19556)
#     ^--- wallet1 (JSON-RPC: locahost:19557, gRPC: localhost:19558)
# dcrd2 (JSON-RPC: localhost:19566)
#     ^--- wallet2 (JSON-RPC: locahost:19567, gRPC: None)

set -e

SESSION="dcrd-simnet-nodes"
NODES_ROOT=${DCR_SIMNET_ROOT:-${HOME}/dcrdsimnetnodes}
RPCUSER="USER"
RPCPASS="PASS"
WALLET_SEED="b280922d2cffda44648346412c5ec97f429938105003730414f10b01e1402eac"
WALLET_MINING_ADDR="SsXciQNTo3HuV5tX3yy4hXndRWgLMRVC7Ah" # NOTE: This must be changed if the seed is changed.
WALLET_XFER_ADDR="SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc" # same as above
WALLET_CREATE_CONFIG="y
n
y
${WALLET_SEED}
"
TSPEND_PRIMARY_WIF=PsUUktzTqNKDRudiz3F4Chh5CKqqmp5W3ckRDhwECbwrSuWZ9m5fk
TSPEND_SECONDARY_WIF=PsUVZDkMHvsH8RmYtCxCWs78xsLU9qAyZyLvV9SJWAdoiJxSFhvFx

if [ -d "${NODES_ROOT}" ] ; then
  rm -R "${NODES_ROOT}"
fi

PRIMARY_DCRD_NAME=dcrd1
SECONDARY_DCRD_NAME=dcrd2
PRIMARY_WALLET_NAME=wallet1
SECONDARY_WALLET_NAME=wallet2
mkdir -p "${NODES_ROOT}/${PRIMARY_DCRD_NAME}"
mkdir -p "${NODES_ROOT}/${SECONDARY_DCRD_NAME}"
mkdir -p "${NODES_ROOT}/${PRIMARY_WALLET_NAME}"
mkdir -p "${NODES_ROOT}/${SECONDARY_WALLET_NAME}"

cat > "${NODES_ROOT}/dcrd.conf" <<EOF
rpcuser=${RPCUSER}
rpcpass=${RPCPASS}
simnet=1
logdir=./log
datadir=./data
debuglevel=TXMP=debug,MINR=debug
txindex=1
EOF

cat > "${NODES_ROOT}/dcrctl.conf" <<EOF
rpcuser=${RPCUSER}
rpcpass=${RPCPASS}
simnet=1
EOF

cat > "${NODES_ROOT}/wallet.conf" <<EOF
username=${RPCUSER}
password=${RPCPASS}
simnet=1
logdir=./log
appdata=./data
pass=123
enablevoting=1
EOF

cd ${NODES_ROOT} && tmux -2 new-session -d -s $SESSION

################################################################################
# Setup the primary dcrd node
################################################################################

PRIMARY_DCRD_P2P=127.0.0.1:19555
PRIMARY_DCRD_RPC=127.0.0.1:19556
tmux rename-window -t $SESSION:0 "${PRIMARY_DCRD_NAME}"
tmux split-window -v
tmux select-pane -t 0
tmux send-keys "cd ${NODES_ROOT}/${PRIMARY_DCRD_NAME}" C-m
tmux send-keys "dcrd -C ../dcrd.conf --listen ${PRIMARY_DCRD_P2P} --miningaddr=${WALLET_MINING_ADDR}" C-m
tmux resize-pane -D 15
tmux select-pane -t 1
tmux send-keys "cd ${NODES_ROOT}/${PRIMARY_DCRD_NAME}" C-m

cat > "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/ctl" <<EOF
#!/usr/bin/env bash
dcrctl -C ../dcrctl.conf "\$@"
EOF
chmod +x "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/ctl"

# Script to mine a specified number of blocks with a delay in between them
# Defaults to 1 block
cat > "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/mine" <<EOF
#!/usr/bin/env bash
NUM=1
case \$1 in
  ''|*[!0-9]*)  ;;
  *) NUM=\$1 ;;
esac

for i in \$(seq \$NUM) ; do
  ./ctl generate 1
  sleep 1
done
EOF
chmod +x "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/mine"
sleep 3
tmux send-keys "./ctl generate 32" C-m

################################################################################
# Setup the primary wallet
################################################################################

PRIMARY_WALLET_RPC=127.0.0.1:19557
PRIMARY_WALLET_GRPC=127.0.0.1:19558
tmux new-window -t $SESSION:1 -n "${PRIMARY_WALLET_NAME}"
tmux split-window -v
tmux select-pane -t 0
tmux resize-pane -D 15
tmux send-keys "cd ${NODES_ROOT}/${PRIMARY_WALLET_NAME}" C-m
tmux send-keys "echo \"${WALLET_CREATE_CONFIG}\" | dcrwallet -C ../wallet.conf --create; tmux wait-for -S ${PRIMARY_WALLET_NAME}" C-m
tmux wait-for ${PRIMARY_WALLET_NAME}
tmux send-keys "dcrwallet -C ../wallet.conf --enableticketbuyer --ticketbuyer.limit=10" C-m
tmux select-pane -t 1
tmux send-keys "cd ${NODES_ROOT}/${PRIMARY_WALLET_NAME}" C-m

cat > "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/ctl" <<EOF
#!/usr/bin/env bash
dcrctl -C ../dcrctl.conf --wallet -c ./data/rpc.cert "\$@"
EOF
chmod +x "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/ctl"

# Script to manually purchase tickets
cat > "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/tickets" <<EOF
#!/usr/bin/env bash
NUM=1
case \$1 in
  ''|*[!0-9]*) ;;
  *) NUM=\$1 ;;
esac

./ctl purchaseticket default 999999 1 \`./ctl getnewaddress\` \$NUM
EOF
chmod +x "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/tickets"

# Script to transfer funds with a specified fee rate
# Defaults to a fee rate of 0.0001
cat > "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/xfer" <<EOF
#!/usr/bin/env bash
DEFAULTFEE=0.0001
FEE=\$DEFAULTFEE
case \$1 in
  ''|*[!0-9\.]*) ;;
  *) FEE=\$1 ;;
esac
if [ "\$FEE" != "\$DEFAULTFEE" ]; then
	./ctl settxfee \$FEE
fi
./ctl sendtoaddress ${WALLET_XFER_ADDR} 0.1
if [ "\$FEE" != "\$DEFAULTFEE" ]; then
	./ctl settxfee \$DEFAULTFEE
fi
EOF
chmod +x "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/xfer"

sleep 1
tmux send-keys "./ctl importprivkey ${TSPEND_PRIMARY_WIF} imported false; ./ctl importprivkey ${TSPEND_SECONDARY_WIF} imported false" C-m

################################################################################
# Setup the serially connected secondary dcrd node
################################################################################

SECONDARY_DCRD_P2P=127.0.0.1:19565
SECONDARY_DCRD_RPC=127.0.0.1:19566
cat > "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/ctl" <<EOF
#!/usr/bin/env bash
dcrctl -C ../dcrctl.conf -s ${SECONDARY_DCRD_RPC} "\$@"
EOF
chmod +x "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/ctl"

cp "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/mine" "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/"
chmod +x "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/mine"

# Script to force reorg
cat > "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/reorg" <<EOF
#!/usr/bin/env bash
./ctl node remove ${PRIMARY_DCRD_P2P}
./mine
cd "${NODES_ROOT}/${PRIMARY_DCRD_NAME}"
./mine 2
cd "${NODES_ROOT}/${SECONDARY_DCRD_NAME}"
./ctl node connect ${PRIMARY_DCRD_P2P} perm
EOF
chmod +x "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/reorg"

tmux new-window -t $SESSION:2 -n "${SECONDARY_DCRD_NAME}"
tmux split-window -v
tmux select-pane -t 0
tmux resize-pane -D 15
tmux send-keys "cd ${NODES_ROOT}/${SECONDARY_DCRD_NAME}" C-m
tmux send-keys "dcrd -C ../dcrd.conf --listen ${SECONDARY_DCRD_P2P} --rpclisten ${SECONDARY_DCRD_RPC} --connect ${PRIMARY_DCRD_P2P}  --miningaddr=${WALLET_MINING_ADDR}" C-m
tmux select-pane -t 1
tmux send-keys "cd ${NODES_ROOT}/${SECONDARY_DCRD_NAME}" C-m

################################################################################
# Setup the secondary wallet
################################################################################

SECONDARY_WALLET_RPC=127.0.0.1:19567
tmux new-window -t $SESSION:3 -n "${SECONDARY_WALLET_NAME}"
tmux split-window -v
tmux select-pane -t 0
tmux resize-pane -D 15
tmux send-keys "cd ${NODES_ROOT}/${SECONDARY_WALLET_NAME}" C-m
tmux send-keys "echo \"${WALLET_CREATE_CONFIG}\" | dcrwallet -C ../wallet.conf --create; tmux wait-for -S ${SECONDARY_WALLET_NAME}" C-m
tmux wait-for ${SECONDARY_WALLET_NAME}
tmux send-keys "dcrwallet -C ../wallet.conf --rpcconnect=${SECONDARY_DCRD_RPC} --rpclisten=${SECONDARY_WALLET_RPC} --nogrpc" C-m
tmux select-pane -t 1
tmux send-keys "cd ${NODES_ROOT}/${SECONDARY_WALLET_NAME}" C-m

cat > "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/ctl" <<EOF
#!/usr/bin/env bash
dcrctl -C ../dcrctl.conf -c ./data/rpc.cert -s ${SECONDARY_WALLET_RPC} "\$@"
EOF
chmod +x "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/ctl"

cp "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/tickets" "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/"
chmod +x "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/tickets"

cp "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/xfer" "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/"
chmod +x "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/xfer"

sleep 1
tmux send-keys "./ctl importprivkey ${TSPEND_PRIMARY_WIF} imported false; ./ctl importprivkey ${TSPEND_SECONDARY_WIF} imported false" C-m

################################################################################
# Setup helper script to stop everything
################################################################################

cat > "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/stopall" <<EOF
#!/usr/bin/env bash
function countdown {
  secs=\$1
  msg=\$2
  while [ \$secs -gt 0  ]; do
    echo -ne "Seconds \$msg: \$secs\033[0K\r"
    sleep 1
    : \$((secs--))
  done
}

cd "${NODES_ROOT}/${PRIMARY_DCRD_NAME}"
./ctl stop 2>/dev/null
cd "${NODES_ROOT}/${PRIMARY_WALLET_NAME}"
./ctl stop 2>/dev/null
cd "${NODES_ROOT}/${SECONDARY_DCRD_NAME}"
./ctl stop 2>/dev/null
cd "${NODES_ROOT}/${SECONDARY_WALLET_NAME}"
./ctl stop 2>/dev/null

DELAY=3
countdown \$DELAY "until shutdown"
tmux kill-session -t $SESSION
EOF
cp "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/stopall" "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/"
cp "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/stopall" "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/"
cp "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/stopall" "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/"
chmod +x "${NODES_ROOT}/${PRIMARY_DCRD_NAME}/stopall"
chmod +x "${NODES_ROOT}/${SECONDARY_DCRD_NAME}/stopall"
chmod +x "${NODES_ROOT}/${PRIMARY_WALLET_NAME}/stopall"
chmod +x "${NODES_ROOT}/${SECONDARY_WALLET_NAME}/stopall"

################################################################################
# Attach
################################################################################

tmux select-window -t $SESSION:0
tmux attach-session -t $SESSION
