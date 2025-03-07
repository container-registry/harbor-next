#!/bin/bash

envFile="/envFile"

# Check if the file exists
if [ ! -f "$envFile" ]; then
  echo "Environment file not found: $envFile"
  exit 1
fi

# Read the file and export each line as an environment variable
while IFS='=' read -r key value; do
  # Ignore empty lines or comments
  if [ -z "$key" ] || [ "${key:0:1}" = "#" ]; then
    continue
  fi

  # Export the environment variable
  export "$key"="$value"

  # print the variable to verify it's set
  echo "Set $key=$value"
done < "$envFile"

# Execute the command (passed as $1 arg)
echo "Executing: $1 with debug enabled at port: $2"

# Start the dlv process in the background
/go/bin/dlv exec --headless --listen localhost:$2 "$1" &

# Execute the second command directly
eval "$1"
