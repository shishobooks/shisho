// The Shisho app for Android (ADR 0021). For now it holds only the API
// types generated from the Go structs and their decode tests; the app
// module joins it with the Android app.
pluginManagement {
    repositories {
        gradlePluginPortal()
        mavenCentral()
    }
}

dependencyResolutionManagement {
    repositories {
        mavenCentral()
    }
}

rootProject.name = "shisho"
include(":api")
