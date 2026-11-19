# The Kotlin backend for Cloud Run. Build from the repository root, which holds the data the build reads (src/lib/packs,
# src/messages/ja/format.json); kotlin/Dockerfile.dockerignore keeps everything else out of the context:
#   docker build -f kotlin/Dockerfile -t typle-kotlin .
FROM gradle:8.14-jdk21 AS build
WORKDIR /w
COPY src/lib/packs/samples.json src/lib/packs/countries.json src/lib/packs/
COPY src/lib/packs/commands/ src/lib/packs/commands/
COPY src/messages/ja/format.json src/messages/ja/
COPY kotlin/ kotlin/
WORKDIR /w/kotlin
RUN ./gradlew installDist --no-daemon -q

FROM eclipse-temurin:21-jre
COPY --from=build /w/kotlin/build/install/typle-kotlin /app
# A plain JVM (no native image, no CRaC), allowed most of the instance's memory rather than the default quarter.
ENV JAVA_OPTS="-XX:MaxRAMPercentage=75"
ENV PORT=8080
EXPOSE 8080
CMD ["/app/bin/typle-kotlin"]
