docker build -f dockerfile/runner_go/Dockerfile -t rin_go --progress plain .
docker build -f dockerfile/runner_c/Dockerfile -t rin_c --progress plain .
docker build -f dockerfile/runner_cpp/Dockerfile -t rin_cpp --progress plain .