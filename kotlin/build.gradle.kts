// The Kotlin backend of the backend comparison (docs/superpowers/specs/2026-11-19-backend-compare-design.md).
plugins {
    kotlin("jvm") version "2.1.21"
    application
}

repositories {
    mavenCentral()
}

kotlin {
    jvmToolchain(21)
}

dependencies {
    implementation("io.ktor:ktor-server-netty:3.1.3")
    implementation("com.zaxxer:HikariCP:6.3.0")
    implementation("org.postgresql:postgresql:42.7.5")
    implementation("org.slf4j:slf4j-simple:2.0.17")
    testImplementation(kotlin("test"))
    testImplementation("org.junit.jupiter:junit-jupiter:5.12.2")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

application {
    mainClass.set("typle.MainKt")
}

// The app's data is read from the repository at build time, never copied by hand: the official lists of
// src/lib/packs (in the order of commands/manifest.json) and the base language's name of the built-in device.
val repository = rootDir.parentFile
val copyAppData by tasks.registering(Copy::class) {
    into(layout.buildDirectory.dir("app-data/app-data"))
    from(repository.resolve("src/lib/packs")) {
        include("samples.json", "countries.json", "commands/*.json")
    }
    from(repository.resolve("src/messages/ja/format.json"))
}
sourceSets.main {
    resources.srcDir(copyAppData.map { layout.buildDirectory.dir("app-data").get() })
}

tasks.test {
    useJUnitPlatform()
    // The tests read the shared fixtures of the repository (tests/fixtures) in place.
    systemProperty("typle.repository", repository.absolutePath)
    environment("TYPLE_TEST_DATABASE_URL", System.getenv("TYPLE_TEST_DATABASE_URL") ?: "")
    testLogging {
        events("passed", "skipped", "failed")
        exceptionFormat = org.gradle.api.tasks.testing.logging.TestExceptionFormat.FULL
    }
}
