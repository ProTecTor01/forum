#!/bin/bash

container_name="forum"
image_name="forum-image"

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
docker rmi $image_name 2>/dev/null || true; print_log $GREEN "cleared image" $ORANGE $image_name
docker build $REBUILD_FLAG -t $image_name .; print_log $GREEN "image created" $ORANGE $image_name

docker ps -a --filter "ancestor=$image_name" -q | xargs -r docker rm; print_log $GREEN "container cleared" $ORANGE $container_name

print_log $GREEN "running" $ORANGE $container_name
docker run --rm -it -p $1:8080 -v .:/forum/src --name $container_name $image_name
docker rmi $image_name
print_log $GREEN "complete"
