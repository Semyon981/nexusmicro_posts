 pipeline {
    agent {
        kubernetes {
            yaml '''
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: go
    image: golang:latest
    args:
    - infinity
    command:
    - sleep
  - name: dind
    image: docker:dind
    command:
    - "dockerd-entrypoint.sh"
    imagePullPolicy: "IfNotPresent"
    securityContext:
      privileged: true
    tty: false
'''
            defaultContainer 'go'
        }
    }

   environment {
        DEPLOYPATH = "apps/nexusmicro/posts/dep.yaml"
        IMAGE = "cr.selcloud.ru/nexushub/nexusmicro_posts"
        DISCORD_WEBHOOK = credentials('discord-webhook')
    }

    stages {
        stage('Build app'){
            steps {
                sh 'make build'
            }
        }

        stage('Buid docker image'){
          steps{
            container('dind') {
              script {
                sh(returnStdout: true, script: 'docker build -t ' + env.IMAGE + ':${BUILD_NUMBER} .')
              }
            }   
          }
        }

        stage('Pushing to registry'){
          when {
              branch 'main'
          }
          steps{
            container('dind'){
              withCredentials([usernamePassword(
               credentialsId: 'regcred', 
               usernameVariable: 'USERNAME', 
               passwordVariable: 'PASSWORD')
              ]) {
                  sh 'echo $PASSWORD | docker login cr.selcloud.ru -u $USERNAME --password-stdin'
              }
              sh(returnStdout: true, script: 'docker push ' + env.IMAGE + ':${BUILD_NUMBER}')
            }
          }
        }

        stage('Trigger manifest update') {      
            when {
                branch 'main'
            }
            steps {
                build job: "kubernetesmanifest", parameters: [
                    string(name: 'TAG', value: env.BUILD_NUMBER), 
                    string(name: 'DEPLOYPATH', value: env.DEPLOYPATH), 
                    string(name: 'IMAGE', value: env.IMAGE)
                ]
            }  
        }
    }
    
    post {
      failure {
          script {
              def blueOceanLink = env.RUN_DISPLAY_URL
              def scmWebUrl = env.CHANGE_URL
              
              discordSend(
                  title: 'Nexusmicro_storage build failed',
                  description: "Ссылка с проблемой: ${blueOceanLink}",
                  footer: '',
                  image: 'https://i.ytimg.com/vi/6KTiQK3MgNM/maxresdefault.jpg?sqp=-oaymwEmCIAKENAF8quKqQMa8AEB-AHUBoAC4AOKAgwIABABGD0gUyhlMA8=&amp;rs=AOn4CLCe3nxrQbIlXI6VeUfwBtrKjkhOtw',
                  link: '',
                  result: 'FAILURE',
                  scmWebUrl: scmWebUrl,
                  thumbnail: 'https://i4.stat01.com/2/3406/134055684/afacdb/kepka-ostrye-kozyrki-shelbi.jpg',
                  webhookURL: env.DISCORD_WEBHOOK
              )
          }
      }
    }
}
