buildscript {
    repositories {
        google()
        mavenCentral()
    }
    dependencies {
        classpath("com.android.tools.build:gradle:9.1.1")
        // pins the Kotlin version that AGP's built-in Kotlin support uses
        classpath(kotlin("gradle-plugin", version = "2.4.0"))
        classpath("org.jetbrains.kotlin:compose-compiler-gradle-plugin:2.4.0")
        classpath("org.jetbrains.kotlin:kotlin-serialization:2.4.0")
    }
}
