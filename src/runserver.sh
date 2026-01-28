#!/bin/bash

container_name="forum"
image_name="forum-image"
script_dir="$(cd "$(dirname "$0")" && pwd)"
db_mount="$script_dir/assets/database"

GREEN="\033[1;38;2;0;255;0m"
ORANGE="\033[1;38;2;255;128;0m"

print_log(){
    color1=$1
    text=$2
    color2=$3
    what=$4
    printf "$1$2\033[m $3$4\033[m\n"
}

if [ -z "$1" ]; then
  echo "./runserver.sh <ip:port> [--rebuild]"
  echo "  --rebuild  Force rebuild without cache"
  exit 1
fi

REBUILD_FLAG=""
if [ "$2" == "--rebuild" ]; then
  REBUILD_FLAG="--no-cache"
  print_log $GREEN "rebuilding" $ORANGE "without cache"
fi

printf "\n"
docker build $REBUILD_FLAG -t $image_name .; print_log $GREEN "image created" $ORANGE $image_name

mkdir -p "$db_mount"

if docker ps -a --format "{{.Names}}" | grep -qx "$container_name"; then
  if docker ps --format "{{.Names}}" | grep -qx "$container_name"; then
    docker stop "$container_name" >/dev/null
  fi
  docker rm "$container_name" >/dev/null
fi

print_log $GREEN "running" $ORANGE $container_name
docker run -it -p $1:8080 -e DB_PATH=/forum/assets/database/forum.db -v "$db_mount":/forum/assets/database -v "$script_dir":/forum/src --name $container_name $image_name

print_log $GREEN "containers" $ORANGE "(docker ps -a)"
docker ps -a
print_log $GREEN "images" $ORANGE "(docker images)"
docker images
print_log $GREEN "complete" $ORANGE ""
