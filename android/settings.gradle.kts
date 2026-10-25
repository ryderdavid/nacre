pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositories {
        google()
        mavenCentral()
    }
}
rootProject.name = "Nacre"

// A fresh checkout or a nacred worktree has no local.properties (it is not
// committed), so with no ANDROID_HOME the build can't find the SDK. Point it
// at Android Studio's default SDK location when one is there.
run {
    val props = file("local.properties")
    val fromEnv = listOf("ANDROID_HOME", "ANDROID_SDK_ROOT").any { !System.getenv(it).isNullOrBlank() }
    val sdk = File(System.getProperty("user.home"), "Library/Android/sdk")
    if (!fromEnv && !props.exists() && sdk.isDirectory) {
        props.writeText("sdk.dir=${sdk.absolutePath}\n")
    }
}
include(":app")
