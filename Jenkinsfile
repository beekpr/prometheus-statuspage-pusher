pipeline {
    agent {
        docker {
            image 'quay.io/beekeeper/golang-ci:1.17'
            args '-v $HOME/.gocache:/.cache -v $HOME/.gomod:/go/pkg/mod -v /var/run/docker.sock:/var/run/docker.sock -v /usr/bin/docker:/usr/bin/docker --user=root'
        }
    }

    options { skipStagesAfterUnstable() }

    stages {
        stage("Build") {
            steps {
                sh 'make build'
            }
        }
        stage("Publish") {
            when {
                anyOf {
                    branch 'develop'
                    branch 'master'
                }
            }
            steps {
                script {
                    VERSION = VersionNumber versionNumberString: '${GIT_BRANCH}-${BUILD_DATE_FORMATTED, "yyyy.MM.dd-HH.mm.ss"}', worstResultForIncrement: 'SUCCESS'
                    sh """
                    echo ${VERSION}
                    git tag -a ${VERSION} -m \"Release ${VERSION}\n\nProudly presented by ${NODE_NAME}\"
                    """
                    IMG = "quay.io/beekeeper/prometheus-statuspage-pusher"
                    DIMG = docker.build IMG
                    docker.withRegistry('https://quay.io', '2979C139-2D08-4159-B777-43F4ECF83DDE') {
                        DIMG.push("${VERSION}")
                    }
                }
            }
        }
        stage("Deploy (Global Services)") {
            when {
                anyOf {
                    branch 'master'
                }
            }
            steps {
                build job: '../../services/prometheus-statuspage-pusher/global-services-prometheus-statuspage-pusher-deploy', parameters: [[$class: 'StringParameterValue', name: 'BKPR_VERSION', value: "${VERSION}"]]
            }
        }
    }
    post {
        cleanup {
            cleanWs()
        }
    }
}
