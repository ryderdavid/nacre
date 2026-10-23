plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
}

android {
    namespace = "com.ryderdavid.nacre"
    compileSdk = 37

    defaultConfig {
        applicationId = "com.ryderdavid.nacre"
        minSdk = 31
        targetSdk = 36
        versionCode = 2
        versionName = "0.2.0"
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            // sideloaded personal build: sign with the debug key until a release key exists
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        resources.excludes += setOf("META-INF/{AL2.0,LGPL2.1}", "META-INF/DEPENDENCIES", "META-INF/INDEX.LIST", "META-INF/*.kotlin_module")
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.core:core-splashscreen:1.2.0")
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("com.google.android.material:material:1.14.0")

    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    // Material 3 Expressive (motion scheme, loading indicator, button groups, floating toolbars)
    implementation("androidx.compose.material3:material3:1.5.0-beta02")
    implementation("androidx.compose.material3:material3-adaptive-navigation-suite:1.5.0-beta02")
    implementation("androidx.compose.material3.adaptive:adaptive:1.3.0")
    implementation("androidx.compose.material:material-icons-extended:1.7.8")
    implementation("androidx.compose.ui:ui-tooling-preview")
    debugImplementation("androidx.compose.ui:ui-tooling")

    implementation("androidx.window:window:1.5.1")
    implementation("androidx.navigation:navigation-compose:2.10.2")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.11.0")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.11.0")
    implementation("androidx.lifecycle:lifecycle-process:2.11.0")
    implementation("androidx.datastore:datastore-preferences:1.3.0")

    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.11.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.12.0")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("com.squareup.okhttp3:okhttp-sse:4.12.0")

    // Claude API (official SDK)
    implementation("com.anthropic:anthropic-java:2.75.0")

    implementation("com.mikepenz:multiplatform-markdown-renderer-m3:0.46.0")
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")

    testImplementation("junit:junit:4.13.2")
    testImplementation(kotlin("test"))
    testImplementation("org.jetbrains.kotlin:kotlin-test-junit")
}
