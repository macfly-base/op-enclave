# ---- Stage 1: Build op-node ----
FROM golang:1.22 AS op-node

WORKDIR /app
ENV REPO=https://github.com/ethereum-optimism/optimism.git
ENV COMMIT=d1ccc976f7c410fede45fd561b8900c077f22029

RUN git clone --depth 1 --branch main $REPO . && \
    git checkout $COMMIT

RUN curl -sSfL 'https://just.systems/install.sh' | bash -s -- --to /usr/local/bin && \
    cd op-node && just op-node

# ---- Stage 2: Build op-enclave ----
FROM golang:1.22 AS op-enclave

WORKDIR /app

COPY op-enclave/go.mod op-enclave/go.sum op-enclave/
COPY go.mod go.sum ./
RUN go mod download

COPY bindings/ bindings/
COPY op-enclave/ op-enclave/
COPY op-batcher/ op-batcher/
COPY op-da/ op-da/
COPY op-proposer/ op-proposer/

RUN cd op-enclave && go build -o ../bin/op-enclave ./cmd/enclave && \
    go build -o bin/op-batcher ./op-batcher/cmd && \
    go build -o bin/op-proposer ./op-proposer/cmd && \
    go build -o bin/op-da ./op-da/cmd

# ---- Stage 3: Build op-geth ----
FROM golang:1.22 AS op-geth

WORKDIR /app
ENV REPO=https://github.com/ethereum-optimism/op-geth.git
ENV COMMIT=928070c7fc097362ed2d40a4f72889ba91544931

RUN git clone --depth 1 --branch main $REPO . && \
    git checkout $COMMIT

RUN go run build/ci.go install -static ./cmd/geth

# ---- Stage 4: Build op-reth ----
FROM ubuntu:22.04 AS op-reth

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    git libclang-dev pkg-config curl build-essential && \
    rm -rf /var/lib/apt/lists/*

RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

ENV REPO=https://github.com/paradigmxyz/reth.git
ENV VERSION=v1.1.0
ENV COMMIT=1ba631ba9581973e7c6cadeea92cfe1802aceb4a

RUN git clone --depth 1 --branch $VERSION $REPO . && \
    git checkout $COMMIT

RUN cargo build --release --bin op-reth --features jemalloc,asm-keccak,optimism \
    --manifest-path crates/optimism/bin/Cargo.toml

# ---- Final Stage: Production Image ----
FROM ubuntu:22.04

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends curl jq && \
    rm -rf /var/lib/apt/lists/*

COPY --from=op-node /app/op-node/bin/op-node ./
COPY --from=op-geth /app/build/bin/geth ./
COPY --from=op-reth /app/target/release/op-reth ./reth
COPY --from=op-enclave /app/bin/op-enclave ./
COPY --from=op-enclave /app/bin/op-batcher ./
COPY --from=op-enclave /app/bin/op-proposer ./
COPY --from=op-enclave /app/bin/op-da ./

COPY testnet/entrypoint-*.sh ./
COPY deployments/ deployments/

ENTRYPOINT ["/app/entrypoint-node.sh"]
