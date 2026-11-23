FROM ubuntu:24.04@sha256:9c7f200c07e3d4a03e7249d2c16bf66a51120fb876aa2242cf31451412543289

RUN usermod --login vscode --home /home/vscode --move-home ubuntu \
    && groupmod --new-name vscode ubuntu

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl openssh-client procps python3 sudo util-linux \
        zsh=5.9-6ubuntu2 \
    && rm -rf /var/lib/apt/lists/*

COPY install-tools.sh /tmp/pocketpull-install-tools.sh
RUN bash /tmp/pocketpull-install-tools.sh && rm /tmp/pocketpull-install-tools.sh
COPY shell-history.sh /etc/profile.d/pocketpull-history.sh

ENV PATH="/usr/local/go/bin:${PATH}"
