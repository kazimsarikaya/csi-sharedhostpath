#!/bin/bash
echo "running post init scripts if exits"

if [[ -d ${HOME}/post-init.d ]]; then
  echo "post init scripts found"
  for sql in $(ls ${HOME}/post-init.d/*.sql); do
    cat $sql | psql
  done
fi
