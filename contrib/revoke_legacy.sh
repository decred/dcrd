#!/bin/sh

# revoke_legacy.sh is a one-time script to be used after the automatic
# revocations consensus change has been activated.  It creates ticket revocation
# transactions for all of the legacy missed and expired tickets that were never
# revoked.
#
# Prerequisites for running this script:
#  - Must have dcrd running with indexes enabled and fully synced
#  - Must have dcrdata running and fully synced
#  - Must have psql available in the system path to execute queries against the
#    dcrdata database
#  - Must have dcrctl available in the system path

# network is the network to run against.  Change to testnet3 for testnet.
network="mainnet"

# batch_size is the number of ticket revocation transactions to create per
# block.
batch_size=5

# dcrctl_cmd is the base dcrctl command.
dcrctl_cmd="dcrctl"
if [ $network == "testnet3" ]; then
  dcrctl_cmd="${dcrctl_cmd} --testnet"
fi

# dcrdata_db_name is the name of the dcrdata database.
dcrdata_db_name="dcrdata_${network}"

# psql is the base psql command to run including the database user, database
# name, and flags.
psql="psql -U dcrdata -d ${dcrdata_db_name} -XAtc"

# unrevoked_sql_count is the query for fetching the total unrevoked tickets
# count.
unrevoked_sql_count="SELECT count(*) "
unrevoked_sql_count+="FROM misses "
unrevoked_sql_count+="INNER JOIN tickets ON ticket_hash = tx_hash "
unrevoked_sql_count+="WHERE spend_tx_db_id IS NULL"

# unrevoked_sql is the query for fetching the next batch of unrevoked tickets.
unrevoked_sql="SELECT ticket_hash, price "
unrevoked_sql+="FROM misses "
unrevoked_sql+="INNER JOIN tickets ON ticket_hash = tx_hash "
unrevoked_sql+="WHERE spend_tx_db_id IS NULL "
unrevoked_sql+="ORDER BY height, ticket_hash "
unrevoked_sql+="LIMIT ${batch_size}"

# Create ticket revocation transactions in batches per-block until there are no
# unrevoked tickets remaining.
while :
do
  # Fetch the best block height.  Continue if the block height has not changed
  # so that only one batch of revocation transactions are broadcast per block.
  height=$(${psql} "SELECT best_block_height FROM meta")
  if [ -n "$prev_height" ] && [ $height == $prev_height ]; then
    # Sleep in order to only query for the best block height every 5 seconds.
    sleep 5
    continue
  fi

  # Fetch the unrevoked tickets count.  Exit if it is zero since there is
  # nothing left to do.
  unrevoked_count=($(${psql} "${unrevoked_sql_count}"))
  if [ $unrevoked_count == 0 ]; then
    echo "No remaining unrevoked tickets... Done!"
    exit 0
  fi

  # Process the next batch of unrevoked tickets.  The unrevoked tickets are
  # queried each block (rather than fetching them once up front) since
  # revocations that aren't included in a block will be dropped from the
  # mempool.
  unrevoked_misses=($(${psql} "${unrevoked_sql}"))
  status_msg="Processing the next ${#unrevoked_misses[@]} unrevoked tickets... "
  status_msg+="(Best Height: ${height}, Total Unrevoked: ${unrevoked_count})"
  echo $status_msg
  for unrevoked in "${unrevoked_misses[@]}"
  do
    # Get the ticket hash and price from the query result.
    IFS='|' read -ra unrevoked_arr <<< "$unrevoked"
    ticket_hash=${unrevoked_arr[0]}
    price=${unrevoked_arr[1]}

    # Construct the createrawssrtx command.
    json="[{\"amount\":$price,\"txid\":\"${ticket_hash}\",\"vout\":0,\"tree\":1}]"
    create_raw_ssrtx_command="${dcrctl_cmd} createrawssrtx ${json}"
    echo $create_raw_ssrtx_command

    # Run the createrawssrtx command.
    revocation_tx_hex=($($create_raw_ssrtx_command))
    echo $revocation_tx_hex

    # Construct the sendrawtransaction command.
    send_raw_tx_command="${dcrctl_cmd} sendrawtransaction ${revocation_tx_hex}"

    # Run the sendrawtransaction command.
    revocation_tx_hash=($($send_raw_tx_command))
    echo $revocation_tx_hash
  done
  echo "\n"

  prev_height=${height}
done
