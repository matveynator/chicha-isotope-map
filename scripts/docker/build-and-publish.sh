#!/usr/bin/env bash
docker login
docker build -t matveynator/chicha-isotope-map:latest .
docker push matveynator/chicha-isotope-map:latest
